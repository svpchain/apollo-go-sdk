package apollo

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// NonceManager must atomically allocate across all senders for a given network/account.
// lastOnChain is the highest executed nonce, not all in-flight reservations.
type NonceManager interface {
	Next(ctx context.Context, account uint32, lastOnChain uint64) (uint64, error)
}

// NonceAllocator coordinates only callers sharing this instance. It is not a distributed lock.
type NonceAllocator struct {
	mu   sync.Mutex
	last map[uint32]uint64
	now  func() time.Time
}

func NewNonceManager() *NonceAllocator {
	return &NonceAllocator{last: make(map[uint32]uint64), now: time.Now}
}
func (n *NonceAllocator) Next(ctx context.Context, account uint32, lastOnChain uint64) (uint64, error) {
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.last == nil {
		n.last = make(map[uint32]uint64)
	}
	previous := n.last[account]
	if lastOnChain > previous {
		previous = lastOnChain
	}
	if previous == math.MaxUint64 {
		return 0, errors.New("nonce exhausted")
	}
	clock := time.Now()
	if n.now != nil {
		clock = n.now()
	}
	micros := clock.UnixMicro()
	candidate := previous + 1
	if micros > 0 && uint64(micros) > candidate {
		candidate = uint64(micros)
	}
	n.last[account] = candidate
	return candidate, nil
}
