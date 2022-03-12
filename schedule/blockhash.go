package schedule

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

type BlockHashMonitor struct {
	client *rpc.Client
	hash   atomic.Value

	Log      *zap.Logger
	Interval time.Duration
}

// NewBlockHashMonitor creates a new unstarted monitor for recent block hashes.
//
// It also fetches one hash to start out during the lifetime of the given context.
func NewBlockHashMonitor(ctx context.Context, client *rpc.Client) (*BlockHashMonitor, error) {
	monitor := &BlockHashMonitor{
		client:   client,
		Log:      zap.NewNop(),
		Interval: 2 * time.Second,
	}
	if err := monitor.tick(ctx); err != nil {
		return nil, fmt.Errorf("failed to get initial recent block hash: %w", err)
	}
	return monitor, nil
}

// Run the block hash monitor in the background.
func (b *BlockHashMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(b.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx, cancel := context.WithTimeout(ctx, b.Interval)
			if err := b.tick(tickCtx); err != nil {
				b.Log.Warn("Failed to get recent block hash", zap.Error(err))
			}
			cancel()
		}
	}
}

func (b *BlockHashMonitor) tick(ctx context.Context) error {
	res, err := b.client.GetRecentBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return err
	}
	if res == nil || res.Value == nil {
		return errors.New("getRecentBlockHash() returned nil")
	}
	b.Log.Debug("Updated recent block hash", zap.Stringer("blockhash", &res.Value.Blockhash))
	b.hash.Store(res.Value)
	return nil
}

// GetRecentBlockHash returns the latest cached "recent blockhash" value.
func (b *BlockHashMonitor) GetRecentBlockHash() *rpc.BlockhashResult {
	return b.hash.Load().(*rpc.BlockhashResult)
}
