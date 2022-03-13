package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	Log            *zap.Logger
	Upgrader       websocket.Upgrader
	Handler        Handler
	ReadTimeout    time.Duration // max time client can spend between creating a request and finish uploading it
	MaxRequestSize uint
}

func NewServer(h Handler) *Server {
	return &Server{
		Log: zap.NewNop(),
		Upgrader: websocket.Upgrader{
			HandshakeTimeout: 5 * time.Second,
		},
		Handler:        h,
		ReadTimeout:    3 * time.Second,
		MaxRequestSize: 128000,
	}
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		s.ServeWebSocket(rw, req)
	case http.MethodPost:
		s.ServePOST(rw, req)
	case http.MethodOptions:
		rw.Header().Set("allow", "OPTIONS, GET, POST")
		rw.Header().Set("access-control-request-method", "OPTIONS, GET, POST")
		rw.Header().Set("access-control-request-headers", "content-type")
		rw.WriteHeader(http.StatusNoContent)
	default:
		http.Error(rw, "Only JSON-RPC 2.0 over HTTP and WebSocket supported", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ServePOST(rw http.ResponseWriter, req *http.Request) {
	// Read request.
	data, err := io.ReadAll(io.LimitReader(req.Body, int64(s.MaxRequestSize)))
	if err != nil {
		return
	}
	reqs, isBatch, err := ParseRequest(data)
	if err != nil {
		http.Error(rw, "Bad request", http.StatusBadRequest)
		return
	}
	// Execute requests.
	respData, err := HandleRequests(req.Context(), s.Handler, nil, reqs, isBatch)
	if err != nil {
		s.Log.Error("Failed to marshal results", zap.Error(err))
		http.Error(rw, "internal server error", http.StatusInternalServerError)
		return // irrecoverable error
	}
	rw.Header().Set("content-type", "application/json; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(respData)
}

func (s *Server) ServeWebSocket(rw http.ResponseWriter, req *http.Request) {
	conn, err := s.Upgrader.Upgrade(rw, req, http.Header{})
	if err != nil {
		return
	}
	newServerConn(conn, s.getLog(req), s).run(req.Context())
}

func (s *Server) getLog(req *http.Request) *zap.Logger {
	return s.Log.With(zap.String("http.client", req.RemoteAddr))
}

// serverConn manages the server-side of a single connection.
type serverConn struct {
	conn   *websocket.Conn
	log    *zap.Logger
	server *Server

	outLock sync.RWMutex
	out     chan *websocket.PreparedMessage
	onClose chan struct{}
}

func newServerConn(conn *websocket.Conn, log *zap.Logger, server *Server) *serverConn {
	return &serverConn{
		conn:    conn,
		out:     make(chan *websocket.PreparedMessage),
		log:     log,
		server:  server,
		onClose: make(chan struct{}),
	}
}

func (h *serverConn) run(ctx context.Context) {
	defer h.dropOut()

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return h.writeLoop(ctx)
	})
	group.Go(func() error {
		return h.readLoop(ctx)
	})
	group.Go(func() error {
		defer close(h.onClose)
		<-ctx.Done()
		return nil
	})
	_ = group.Wait()
}

func (h *serverConn) readLoop(ctx context.Context) error {
	defer h.close()
	for {
		// Read and parse request.
		_, rd, err := h.conn.NextReader()
		if err != nil {
			return err
		}
		_ = h.conn.SetReadDeadline(time.Now().Add(h.server.ReadTimeout))
		data, err := io.ReadAll(io.LimitReader(rd, int64(h.server.MaxRequestSize)))
		if err != nil {
			return err
		}
		_ = h.conn.SetReadDeadline(time.Time{}) // no limit

		reqs, isBatch, err := ParseRequest(data)
		if err != nil {
			h.writeMessage(ctx, NewParseErrorResponse(err))
			continue
		}
		// Execute requests.
		respData, err := HandleRequests(ctx, h.server.Handler, h, reqs, isBatch)
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err) // irrecoverable error
		}
		if len(respData) > 0 {
			h.writeMessage(ctx, json.RawMessage(respData))
		}
	}
}

func (h *serverConn) writeMessage(ctx context.Context, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		h.log.Error("Failed to marshal message", zap.Error(err))
		return
	}
	msg, err := websocket.NewPreparedMessage(websocket.TextMessage, buf)
	if err != nil {
		h.log.Error("Failed to prepare WebSocket message", zap.Error(err))
		return
	}

	select {
	case <-ctx.Done():
	case h.out <- msg:
	}
}

func (h *serverConn) writeLoop(ctx context.Context) error {
	defer h.close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-h.out:
			if !ok {
				return nil
			}
			if err := h.conn.WritePreparedMessage(msg); err != nil {
				return err
			}
		}
	}
}

func (h *serverConn) close() {
	_ = h.conn.Close()
}

func (h *serverConn) dropOut() {
	out := h.out
	h.outLock.Lock()
	h.out = nil
	h.outLock.Unlock()
	close(out)
}

// AsyncRequestJSONRPC sends a JSON-RPC notification from server to client.
//
// Returns net.ErrClosed if the underlying connection has been closed already.
func (h *serverConn) AsyncRequestJSONRPC(ctx context.Context, method string, params interface{}) error {
	// Encode request to JSON.
	req := Request{
		Version: Version,
		ID:      Null,
		Method:  method,
		Params:  params,
	}
	buf, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("failed to marshal request params: %w", err)
	}

	// Create new WebSocket message.
	msg, err := websocket.NewPreparedMessage(websocket.TextMessage, buf)
	if err != nil {
		return err
	}

	// Blocking send to writer thread.
	h.outLock.RLock()
	defer h.outLock.RUnlock()
	select {
	case <-h.onClose:
		return net.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case h.out <- msg:
		return nil
	}
}
