package jsonrpc

import "context"

type Mux struct {
	handlers map[string]Handler
}

func NewMux() *Mux {
	return &Mux{handlers: make(map[string]Handler)}
}

func (m *Mux) Handle(method string, sub Handler) {
	m.handlers[method] = sub
}

func (m *Mux) HandleFunc(method string, f HandleFunc) {
	m.handlers[method] = f
}

func (m *Mux) ServeJSONRPC(ctx context.Context, req Request, callback Requester) *Response {
	handler := m.handlers[req.Method]
	if handler == nil {
		return NewMethodNotFoundResponse(req.ID)
	}
	return handler.ServeJSONRPC(ctx, req, callback)
}
