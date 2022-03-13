package schedule

import (
	"sync"

	"github.com/gagliardetto/solana-go"
	"go.blockdaemon.com/pyth"
	"go.uber.org/zap"
)

// Buffer collects price update instructions.
type Buffer struct {
	Log *zap.Logger

	lock    sync.Mutex
	updates map[solana.PublicKey]*pyth.Instruction
}

func NewBuffer() *Buffer {
	return &Buffer{
		Log:     zap.NewNop(),
		updates: make(map[solana.PublicKey]*pyth.Instruction),
	}
}

func (b *Buffer) PushUpdate(ins *pyth.Instruction) {
	_, ok := ins.Payload.(*pyth.CommandUpdPrice)
	if !ok {
		return
	}
	accs := ins.Accounts()
	if len(accs) != 3 {
		return
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.updates[accs[1].PublicKey] = ins
}

// Flush removes all queued instructions and places them into an unsigned transaction.
// Returns nil if the buffer is empty.
//
// Updates created earlier than the given minSlot will be removed.
func (b *Buffer) Flush(minSlot uint64) *solana.TransactionBuilder {
	b.lock.Lock()
	defer b.lock.Unlock()

	// TODO(richard): Will fail if payload exceeds MTU, split into multiple txs
	builder := solana.NewTransactionBuilder()
	var updates uint
	for price, insn := range b.updates {
		delete(b.updates, price)
		if b.appendUpdateToBuilder(builder, insn, minSlot) {
			updates++
		}
	}
	if updates == 0 {
		return nil
	}
	return builder
}

func (b *Buffer) appendUpdateToBuilder(builder *solana.TransactionBuilder, insn *pyth.Instruction, minSlot uint64) bool {
	update, ok := insn.Payload.(*pyth.CommandUpdPrice)
	if !ok {
		return false
	}
	if update.PubSlot < minSlot {
		b.Log.Warn("Dropping price update",
			zap.Stringer("price", insn.Accounts()[1].PublicKey),
			zap.Uint64("pub_slot", update.PubSlot),
			zap.Uint64("min_slot", minSlot))
		return false
	}
	builder.AddInstruction(insn)
	return true
}
