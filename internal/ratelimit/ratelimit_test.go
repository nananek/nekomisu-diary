package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestAllow_UnderLimit(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("user1") {
			t.Errorf("attempt %d should be allowed", i+1)
		}
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		l.Allow("user1")
	}
	if l.Allow("user1") {
		t.Error("4th attempt should be denied")
	}
}

func TestAllow_DifferentKeys(t *testing.T) {
	l := New(2, time.Minute)
	l.Allow("a"); l.Allow("a")
	if !l.Allow("b") {
		t.Error("key b should not be affected by key a's attempts")
	}
}

func TestAllow_WindowExpiry(t *testing.T) {
	now := time.Now()
	l := New(2, 10*time.Second)
	l.clock = func() time.Time { return now }

	l.Allow("user1")
	l.Allow("user1")
	if l.Allow("user1") {
		t.Fatal("3rd attempt should be denied")
	}

	// Advance past window
	l.clock = func() time.Time { return now.Add(11 * time.Second) }

	if !l.Allow("user1") {
		t.Error("after window expiry, should be allowed again")
	}
}

func TestReset(t *testing.T) {
	l := New(2, time.Minute)
	l.Allow("user1"); l.Allow("user1")
	l.Reset("user1")
	if !l.Allow("user1") {
		t.Error("after reset, should be allowed again")
	}
}

func TestCleanup_RemovesExpired(t *testing.T) {
	now := time.Now()
	l := New(5, 10*time.Second)
	l.clock = func() time.Time { return now }
	l.Allow("user1")
	l.Allow("user2")

	l.clock = func() time.Time { return now.Add(20 * time.Second) }
	l.Cleanup()

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.attempts) != 0 {
		t.Errorf("expected empty after cleanup, got %d entries", len(l.attempts))
	}
}

func TestConcurrent(t *testing.T) {
	l := New(1000, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); l.Allow("user") }()
	}
	wg.Wait()
	// If no race, we're fine
}
