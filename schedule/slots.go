package schedule

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"go.uber.org/zap"
)

type SlotMonitor struct {
	Log          *zap.Logger
	WebSocketURL string

	updates  chan *ws.SlotsUpdatesResult
	lastSlot uint64
}

func NewSlotMonitor(wsURL string) *SlotMonitor {
	return &SlotMonitor{
		Log:          zap.NewNop(),
		WebSocketURL: wsURL,

		updates: make(chan *ws.SlotsUpdatesResult, 1),
	}
}

func (s *SlotMonitor) Run(ctx context.Context) error {
	defer close(s.updates)
	const retryInterval = 3 * time.Second
	return backoff.Retry(func() error {
		err := s.runConn(ctx)
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return backoff.Permanent(err)
		default:
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil
			}
			s.Log.Error("Stream failed, restarting", zap.Error(err))
			return err
		}
	}, backoff.WithContext(backoff.NewConstantBackOff(retryInterval), ctx))
}

func (s *SlotMonitor) runConn(ctx context.Context) error {
	client, err := ws.Connect(ctx, s.WebSocketURL)
	if err != nil {
		return err
	}
	defer client.Close()

	// Make sure client cannot outlive context.
	go func() {
		defer client.Close()
		<-ctx.Done()
	}()

	sub, err := client.SlotsUpdatesSubscribe()
	if err != nil {
		return err
	}

	// Stream updates.
	for {
		err := s.readNextUpdate(ctx, sub)
		if errors.Is(err, context.Canceled) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

func (s *SlotMonitor) readNextUpdate(ctx context.Context, sub *ws.SlotsUpdatesSubscription) error {
	// If no update comes in within 20 seconds, bail.
	const readTimeout = 20 * time.Second
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	go func() {
		<-ctx.Done()
		// Terminate subscription if above timer has expired.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.Log.Warn("Read deadline exceeded, terminating WebSocket connection",
				zap.Duration("timeout", readTimeout))
			sub.Unsubscribe()
		}
	}()

	// Read next account update from WebSockets.
	update, err := sub.Recv()
	if err != nil {
		return err
	} else if update == nil {
		return nil
	} else if update.Timestamp == nil {
		ts := solana.UnixTimeSeconds(time.Now().Unix())
		update.Timestamp = &ts
	}

	// Only listen for "first shred received" pings for now.
	if update.Type != ws.SlotsUpdatesFirstShredReceived {
		return nil
	}
	atomic.StoreUint64(&s.lastSlot, update.Slot)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.updates <- update:
		s.Log.Debug("Slot update", zap.Uint64("slot", update.Slot))
	default:
		s.Log.Warn("Dropping slot update", zap.Uint64("slot", update.Slot))
	}

	return nil
}

// Updates the single current update channel.
func (s *SlotMonitor) Updates() <-chan *ws.SlotsUpdatesResult {
	return s.updates
}

// Slot returns the slot number that the cluster is currently processing. 0 if unknown.
func (s *SlotMonitor) Slot() uint64 {
	return atomic.LoadUint64(&s.lastSlot)
}
