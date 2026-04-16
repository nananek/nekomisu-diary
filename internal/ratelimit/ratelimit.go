// Package ratelimit provides a simple in-memory sliding-window limiter.
// Suitable for small-group services; not distributed.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu       sync.Mutex
	window   time.Duration
	maxHits  int
	attempts map[string][]time.Time
	// clock allows tests to inject a fake time source.
	clock func() time.Time
}

// New creates a limiter allowing `maxHits` events per sliding `window`.
func New(maxHits int, window time.Duration) *Limiter {
	return &Limiter{
		window:   window,
		maxHits:  maxHits,
		attempts: make(map[string][]time.Time),
		clock:    time.Now,
	}
}

// Allow records an attempt for `key` and returns true if under the limit.
// Returns false when the rate would be exceeded.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock()
	cutoff := now.Add(-l.window)

	times := l.attempts[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.maxHits {
		l.attempts[key] = kept
		return false
	}

	kept = append(kept, now)
	l.attempts[key] = kept
	return true
}

// Reset clears attempts for a given key (e.g. after successful auth).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// Cleanup removes expired entries. Call periodically.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.clock().Add(-l.window)
	for key, times := range l.attempts {
		kept := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(l.attempts, key)
		} else {
			l.attempts[key] = kept
		}
	}
}
