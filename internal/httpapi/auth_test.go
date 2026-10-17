package httpapi

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestLoginLimiterLocksAfterRepeatedFailures(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	const ip = "192.0.2.10"

	for i := 0; i < loginAttemptThreshold; i++ {
		allowed, retry := limiter.allow(ip, now)
		if !allowed {
			t.Fatalf("attempt %d unexpectedly blocked for %s", i+1, retry)
		}
		limiter.recordFailure(ip, now)
		now = now.Add(time.Second)
	}

	allowed, retry := limiter.allow(ip, now)
	if allowed {
		t.Fatal("login attempt should be blocked after the failure threshold")
	}
	if retry <= 0 || retry > loginLockDuration {
		t.Fatalf("unexpected retry duration: %s", retry)
	}

	allowed, _ = limiter.allow(ip, now.Add(loginLockDuration+time.Second))
	if !allowed {
		t.Fatal("login attempt should be allowed after the lock expires")
	}
}

func TestLoginLimiterExpiresIdleEntries(t *testing.T) {
	limiter := newLoginLimiter()
	start := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	limiter.allow("192.0.2.20", start)
	limiter.recordFailure("192.0.2.20", start)

	limiter.allow("192.0.2.21", start.Add(loginEntryIdleTTL+time.Second))

	limiter.Lock()
	defer limiter.Unlock()
	if _, exists := limiter.entries["192.0.2.20"]; exists {
		t.Fatal("idle entry was not cleaned up")
	}
	if len(limiter.entries) != 1 {
		t.Fatalf("expected only the fresh entry to remain, got %d", len(limiter.entries))
	}
}

func TestLoginLimiterBoundsIPCardinality(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

	for i := 0; i < maxLoginLimitEntries+64; i++ {
		ip := "test-ip-" + strconv.Itoa(i)
		limiter.allow(ip, now.Add(time.Duration(i)*time.Nanosecond))
	}

	limiter.Lock()
	defer limiter.Unlock()
	if len(limiter.entries) > maxLoginLimitEntries {
		t.Fatalf("login limiter grew beyond its cap: got %d, cap %d", len(limiter.entries), maxLoginLimitEntries)
	}
}

func TestLoginLimiterConcurrentAccess(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := "concurrent-ip-" + strconv.Itoa(i%8)
			for j := 0; j < 20; j++ {
				at := now.Add(time.Duration(j) * time.Second)
				if allowed, _ := limiter.allow(ip, at); allowed {
					limiter.recordFailure(ip, at)
				}
			}
		}(i)
	}
	wg.Wait()

	limiter.Lock()
	defer limiter.Unlock()
	if len(limiter.entries) > maxLoginLimitEntries {
		t.Fatalf("login limiter grew beyond its cap under concurrency: %d", len(limiter.entries))
	}
}
