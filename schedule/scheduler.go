package schedule

import (
	"context"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"go.blockdaemon.com/pythian/signer"
	"go.uber.org/zap"
)

// Scheduler buffers price updates and submits transactions.
type Scheduler struct {
	Log *zap.Logger

	buffer    *Buffer
	blockhash *BlockHashMonitor
	signer    *signer.Signer
	rpc       *rpc.Client
	wg        sync.WaitGroup
}

// NewScheduler creates a new unstarted scheduler.
func NewScheduler(buffer *Buffer, blockhash *BlockHashMonitor, signer *signer.Signer, rpc *rpc.Client) *Scheduler {
	return &Scheduler{
		Log: zap.NewNop(),

		buffer:    buffer,
		blockhash: blockhash,
		signer:    signer,
		rpc:       rpc,
	}
}

// Run executes the price update scheduler loop.
//
// The provided "slot updates" channel acts as the heart beat that ticks the loop.
// This method will return when the scheduler shuts down.
func (s *Scheduler) Run(ctx context.Context, updates <-chan *ws.SlotsUpdatesResult) {
	defer s.wg.Wait()
	for update := range updates {
		s.tick(ctx, update)
	}
}

func (s *Scheduler) tick(ctx context.Context, update *ws.SlotsUpdatesResult) {
	// Assemble transaction.
	builder := s.buffer.Flush(update.Slot - 32)
	if builder == nil {
		return
	}
	builder.SetFeePayer(s.signer.Pubkey())
	builder.SetRecentBlockHash(s.blockhash.GetRecentBlockHash().Blockhash)
	tx, err := builder.Build()
	if err != nil {
		s.Log.Error("Failed to build transaction", zap.Error(err))
		return
	}

	// Sign transaction.
	if err := s.signer.SignPriceUpdate(tx); err != nil {
		s.Log.Error("Failed to sign transaction", zap.Error(err))
	}

	s.Log.Info("Submitting price update",
		zap.Int("updates", len(tx.Message.Instructions)))

	s.wg.Add(1)
	go s.sendTransaction(ctx, tx)
}

func (s *Scheduler) sendTransaction(ctx context.Context, tx *solana.Transaction) {
	defer s.wg.Done()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	sig, err := s.rpc.SendTransactionWithOpts(ctx, tx, true, rpc.CommitmentProcessed)
	if err != nil {
		s.Log.Error("Failed to send transaction", zap.Error(err))
		return
	}

	s.Log.Info("Sent transaction", zap.Stringer("signature", sig))
}
