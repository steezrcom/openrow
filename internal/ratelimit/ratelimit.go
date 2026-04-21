// Package ratelimit offers a simple per-key token-bucket limiter for HTTP
// handlers. One limiter is kept per key (typically a user or tenant id) so
// one noisy account can't starve the rest.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Keyed is goroutine-safe. Idle keys stick around until Sweep() is called;
// each limiter is a few dozen bytes, so a short-lived instance can skip that.
type Keyed struct {
	rps      rate.Limit
	burst    int
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	lastSeen map[string]time.Time
}

// New returns a limiter that permits `perSecond` events on average with a
// burst allowance. Typical HTTP use: perSecond=0.5 (1 / 2s) with burst=5.
func New(perSecond float64, burst int) *Keyed {
	return &Keyed{
		rps:      rate.Limit(perSecond),
		burst:    burst,
		limiters: map[string]*rate.Limiter{},
		lastSeen: map[string]time.Time{},
	}
}

// Allow reports whether the event for `key` may proceed, and returns the
// suggested retry delay when it can't.
func (k *Keyed) Allow(key string) (bool, time.Duration) {
	k.mu.Lock()
	defer k.mu.Unlock()
	lim, ok := k.limiters[key]
	if !ok {
		lim = rate.NewLimiter(k.rps, k.burst)
		k.limiters[key] = lim
	}
	k.lastSeen[key] = time.Now()
	reservation := lim.Reserve()
	if !reservation.OK() {
		return false, time.Second
	}
	delay := reservation.Delay()
	if delay == 0 {
		return true, 0
	}
	// Not allowed right now — undo the reservation so other callers aren't
	// penalised, and hand back a retry hint.
	reservation.Cancel()
	return false, delay
}

// Sweep drops keys that haven't been seen in `idle`. Callers can run this
// on a ticker if they expect a long-lived process with many keys.
func (k *Keyed) Sweep(idle time.Duration) {
	cutoff := time.Now().Add(-idle)
	k.mu.Lock()
	defer k.mu.Unlock()
	for key, seen := range k.lastSeen {
		if seen.Before(cutoff) {
			delete(k.limiters, key)
			delete(k.lastSeen, key)
		}
	}
}
