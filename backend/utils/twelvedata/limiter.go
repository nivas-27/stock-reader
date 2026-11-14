package twelvedata

import (
	"context"
	"sync"
	"time"
)

const pollWhileYielding = 250 * time.Millisecond

// Limiter is a sliding-window request budget shared by every Twelve Data call.
// Background work may use only max-reserve slots, so interactive calls
// (search, add) never queue behind a long background run.
type Limiter struct {
	mu      sync.Mutex
	max     int
	reserve int
	window  time.Duration
	sent    []time.Time
	waiting int // interactive callers currently waiting
}

func NewLimiter(max, reserve int, window time.Duration) *Limiter {
	return &Limiter{max: max, reserve: reserve, window: window}
}

// Wait blocks until a slot is free or ctx ends.
func (l *Limiter) Wait(ctx context.Context, interactive bool) error {
	if interactive {
		l.mu.Lock()
		l.waiting++
		l.mu.Unlock()
		defer func() {
			l.mu.Lock()
			l.waiting--
			l.mu.Unlock()
		}()
	}

	for {
		delay, ok := l.tryTake(interactive)
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (l *Limiter) tryTake(interactive bool) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	drop := 0
	for drop < len(l.sent) && now.Sub(l.sent[drop]) >= l.window {
		drop++
	}
	l.sent = l.sent[drop:]

	if !interactive && l.waiting > 0 {
		return pollWhileYielding, false
	}

	limit := l.max
	if !interactive {
		limit -= l.reserve
	}
	if len(l.sent) >= limit {
		return l.window - now.Sub(l.sent[0]), false
	}

	l.sent = append(l.sent, now)
	return 0, true
}
