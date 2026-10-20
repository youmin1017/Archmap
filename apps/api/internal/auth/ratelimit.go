package auth

import (
	"sync"
	"time"
)

// Limiter counts login failures per key (ADR-057). A shared store can implement
// it later; one API instance keeps it in memory.
type Limiter interface {
	// Blocked reports whether key is over its limit and, if so, for how long.
	Blocked(key string, now time.Time) (time.Duration, bool)
	Fail(key string, now time.Time)
	Reset(key string)
}

// WindowLimiter allows max failures per fixed window and key.
type WindowLimiter struct {
	max    int
	window time.Duration

	mu      sync.Mutex
	entries map[string]*windowEntry
}

type windowEntry struct {
	start time.Time
	count int
}

func NewWindowLimiter(maxFailures int, window time.Duration) *WindowLimiter {
	return &WindowLimiter{max: maxFailures, window: window, entries: map[string]*windowEntry{}}
}

func (l *WindowLimiter) Blocked(key string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || now.Sub(e.start) >= l.window || e.count < l.max {
		return 0, false
	}
	return e.start.Add(l.window).Sub(now), true
}

func (l *WindowLimiter) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) > 10_000 {
		l.prune(now)
	}
	e, ok := l.entries[key]
	if !ok || now.Sub(e.start) >= l.window {
		l.entries[key] = &windowEntry{start: now, count: 1}
		return
	}
	e.count++
}

func (l *WindowLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *WindowLimiter) prune(now time.Time) {
	for k, e := range l.entries {
		if now.Sub(e.start) >= l.window {
			delete(l.entries, k)
		}
	}
}
