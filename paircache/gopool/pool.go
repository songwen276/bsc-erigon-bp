package pairpool

import (
	"github.com/panjf2000/ants/v2"
	"time"
)

var (
	// Init a instance pool when importing ants.
	defaultPool, _ = ants.NewPool(1000, ants.WithExpiryDuration(5*time.Second))
)

// Logger is used for logging formatted messages.
type Logger interface {
	// Printf must have the same semantics as log.Printf.
	Printf(format string, args ...interface{})
}

// Submit submits a task to pool.
func Submit(task func()) error {
	return defaultPool.Submit(task)
}
