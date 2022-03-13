package schedule

import (
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
}

// NewScheduler creates a new unstarted scheduler.
func NewScheduler(buffer *Buffer, blockhash *BlockHashMonitor, signer *signer.Signer) *Scheduler {
	return &Scheduler{
		Log: zap.NewNop(),

		buffer:    buffer,
		blockhash: blockhash,
		signer:    signer,
	}
}

// Run executes the price update scheduler loop.
//
// The provided "slot updates" channel acts as the heart beat that ticks the loop.
// This method will return when the scheduler shuts down.
func (s *Scheduler) Run(updates <-chan *ws.SlotsUpdatesResult) {
	for update := range updates {
		s.tick(update)
	}
}

func (s *Scheduler) tick(update *ws.SlotsUpdatesResult) {
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
}
