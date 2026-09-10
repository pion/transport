// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package deadline

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeadline(t *testing.T) {
	ctx := t.Context()

	t.Run("Deadline", func(t *testing.T) {
		now := time.Now()

		ctx0, cancel0 := context.WithDeadline(ctx, now.Add(40*time.Millisecond))
		defer cancel0()
		ctx1, cancel1 := context.WithDeadline(ctx, now.Add(60*time.Millisecond))
		defer cancel1()
		d := New()
		d.Set(now.Add(50 * time.Millisecond))

		ch := make(chan byte)
		go sendOnDone(ctx, ctx0.Done(), ch, 0)
		go sendOnDone(ctx, ctx1.Done(), ch, 1)
		go sendOnDone(ctx, d.Done(), ch, 2)

		calls := collectCh(ch, 3, 100*time.Millisecond)
		expectedCalls := []byte{0, 2, 1}
		assert.Equal(t, expectedCalls, calls, "Wrong order of deadline signal")
	})

	t.Run("DeadlineExtend", func(t *testing.T) { //nolint:dupl
		now := time.Now()

		ctx0, cancel0 := context.WithDeadline(ctx, now.Add(40*time.Millisecond))
		defer cancel0()
		ctx1, cancel1 := context.WithDeadline(ctx, now.Add(60*time.Millisecond))
		defer cancel1()
		d := New()
		d.Set(now.Add(50 * time.Millisecond))
		d.Set(now.Add(70 * time.Millisecond))

		ch := make(chan byte)
		go sendOnDone(ctx, ctx0.Done(), ch, 0)
		go sendOnDone(ctx, ctx1.Done(), ch, 1)
		go sendOnDone(ctx, d.Done(), ch, 2)

		calls := collectCh(ch, 3, 100*time.Millisecond)
		expectedCalls := []byte{0, 1, 2}
		assert.Equal(t, expectedCalls, calls, "Wrong order of deadline signal")
	})

	t.Run("DeadlinePretend", func(t *testing.T) { //nolint:dupl
		now := time.Now()

		ctx0, cancel0 := context.WithDeadline(ctx, now.Add(40*time.Millisecond))
		defer cancel0()
		ctx1, cancel1 := context.WithDeadline(ctx, now.Add(60*time.Millisecond))
		defer cancel1()
		d := New()
		d.Set(now.Add(50 * time.Millisecond))
		d.Set(now.Add(30 * time.Millisecond))

		ch := make(chan byte)
		go sendOnDone(ctx, ctx0.Done(), ch, 0)
		go sendOnDone(ctx, ctx1.Done(), ch, 1)
		go sendOnDone(ctx, d.Done(), ch, 2)

		calls := collectCh(ch, 3, 100*time.Millisecond)
		expectedCalls := []byte{2, 0, 1}
		assert.Equal(t, expectedCalls, calls, "Wrong order of deadline signal")
	})

	t.Run("DeadlineCancel", func(t *testing.T) {
		now := time.Now()

		ctx0, cancel0 := context.WithDeadline(ctx, now.Add(40*time.Millisecond))
		defer cancel0()
		d := New()
		d.Set(now.Add(50 * time.Millisecond))
		d.Set(time.Time{})

		ch := make(chan byte)
		go sendOnDone(ctx, ctx0.Done(), ch, 0)
		go sendOnDone(ctx, d.Done(), ch, 1)

		calls := collectCh(ch, 2, 60*time.Millisecond)
		expectedCalls := []byte{0}
		assert.Equal(t, expectedCalls, calls, "Wrong order of deadline signal")
	})
}

func sendOnDone(ctx context.Context, done <-chan struct{}, dest chan byte, val byte) {
	select {
	case <-done:
	case <-ctx.Done():
		return
	}
	dest <- val
}

func collectCh(ch <-chan byte, n int, timeout time.Duration) []byte {
	a := time.After(timeout)
	var calls []byte
	for len(calls) < n {
		select {
		case call := <-ch:
			calls = append(calls, call)
		case <-a:
			return calls
		}
	}

	return calls
}

func TestContext(t *testing.T) { //nolint:cyclop
	t.Run("Cancel", func(t *testing.T) {
		deadline := New()

		select {
		case <-deadline.Done():
			assert.Fail(t, "Deadline unexpectedly done")
		case <-time.After(50 * time.Millisecond):
		}
		assert.NoError(t, deadline.Err())
		deadline.Set(time.Unix(0, 1)) // exceeded
		select {
		case <-deadline.Done():
		case <-time.After(50 * time.Millisecond):
			assert.Fail(t, "Timeout")
		}
		assert.ErrorIs(t, deadline.Err(), context.DeadlineExceeded)
	})
	t.Run("Deadline", func(t *testing.T) {
		d := New()
		t0, expired0 := d.Deadline()
		assert.True(t, t0.IsZero(), "Initial Deadline is expected to be 0")
		assert.False(t, expired0, "Deadline is not expected to be expired at initial state")

		dl := time.Unix(12345, 0)
		d.Set(dl) // exceeded

		t1, expired1 := d.Deadline()
		assert.True(t, t1.Equal(dl), "Initial Deadline is expected to be %v, got %v", dl, t1)
		assert.True(t, expired1, "Deadline is expected to be expired")
	})
}

func assertCanceled(t *testing.T, ctx context.Context, cause error) {
	t.Helper()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		assert.Fail(t, "context was not canceled")

		return
	}
	assert.ErrorIs(t, context.Cause(ctx), cause)
}

func assertLive(t *testing.T, ctx context.Context) {
	t.Helper()

	select {
	case <-ctx.Done():
		assert.Fail(t, "context was unexpectedly canceled")
	default:
	}
	assert.NoError(t, ctx.Err())
}

func TestDeadlineContext(t *testing.T) {
	t.Run("MemoizedAcrossCalls", func(t *testing.T) {
		d := New()
		d.Set(time.Now().Add(time.Hour))

		assert.Equal(t, d.Context(), d.Context(), "Context should be memoized")
	})

	t.Run("ExtendKeepsSameContext", func(t *testing.T) {
		d := New()
		d.Set(time.Now().Add(time.Hour))
		ctx := d.Context()

		d.Set(time.Now().Add(2 * time.Hour))

		assert.Equal(t, ctx, d.Context(), "Extending the deadline should not retire the context")
		assertLive(t, ctx)
	})

	t.Run("CanceledByTimer", func(t *testing.T) {
		d := New()
		d.Set(time.Now().Add(10 * time.Millisecond))

		assertCanceled(t, d.Context(), context.DeadlineExceeded)
	})

	t.Run("CanceledByPastDeadline", func(t *testing.T) {
		d := New()
		ctx := d.Context()
		d.Set(time.Unix(0, 1))

		assertCanceled(t, ctx, context.DeadlineExceeded)
	})

	t.Run("SuspendKeepsContextLive", func(t *testing.T) {
		d := New()
		d.Set(time.Now().Add(time.Hour))
		ctx := d.Context()

		d.Set(time.Time{}) // no deadline

		assert.Equal(t, ctx, d.Context(), "Suspending the deadline should not retire the context")
		assertLive(t, ctx)
	})

	t.Run("ExceededReturnsCanceledContext", func(t *testing.T) {
		d := New()
		d.Set(time.Unix(0, 1))

		assertCanceled(t, d.Context(), context.DeadlineExceeded)
	})

	t.Run("ExceededByTimerReturnsCanceledContext", func(t *testing.T) {
		d := New()
		d.Set(time.Now().Add(10 * time.Millisecond))
		assertCanceled(t, d.Context(), context.DeadlineExceeded)

		assertCanceled(t, d.Context(), context.DeadlineExceeded)
	})

	t.Run("DerivedFromExceededIsCanceled", func(t *testing.T) {
		d := New()
		d.Set(time.Unix(0, 1))

		derived, cancel := context.WithCancel(d.Context())
		defer cancel()

		assertCanceled(t, derived, context.DeadlineExceeded)
	})

	t.Run("SuspendAfterExpiryGivesLiveContext", func(t *testing.T) {
		d := New()
		d.Set(time.Unix(0, 1))
		assertCanceled(t, d.Context(), context.DeadlineExceeded)

		d.Set(time.Time{}) // no deadline

		assertLive(t, d.Context())
	})

	t.Run("NewGenerationAfterExpiry", func(t *testing.T) {
		d := New()
		expired := d.Context()
		d.Set(time.Unix(0, 1))
		assertCanceled(t, expired, context.DeadlineExceeded)

		d.Set(time.Now().Add(time.Hour))
		revived := d.Context()

		assert.NotEqual(t, expired, revived, "Reviving the deadline should mint a new context")
		assertLive(t, revived)
		assertCanceled(t, expired, context.DeadlineExceeded)
	})

	t.Run("DerivedContextIsCanceled", func(t *testing.T) {
		d := New()
		d.Set(time.Now().Add(10 * time.Millisecond))

		derived, cancel := context.WithCancel(d.Context())
		defer cancel()

		assertCanceled(t, derived, context.DeadlineExceeded)
	})

	t.Run("DerivedContextsDoNotSpawnGoroutines", func(t *testing.T) {
		const derived = 100

		d := New()
		d.Set(time.Now().Add(time.Hour))
		parent := d.Context()

		runtime.GC()
		before := runtime.NumGoroutine()

		cancels := make([]context.CancelFunc, 0, derived)
		for range derived {
			_, cancel := context.WithCancel(parent)
			cancels = append(cancels, cancel)
		}
		spawned := runtime.NumGoroutine() - before
		for _, cancel := range cancels {
			cancel()
		}

		// A *cancelCtx parent registers children in a map; anything else costs
		// a goroutine per child.
		assert.Less(t, spawned, derived/10, "Deriving contexts should not spawn goroutines")
	})

	t.Run("ConcurrentContextAndSet", func(t *testing.T) {
		deadline := New()

		var wg sync.WaitGroup
		for range 8 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				for range 500 {
					_ = deadline.Context().Err()
				}
			}()
			go func() {
				defer wg.Done()
				for range 500 {
					deadline.Set(time.Unix(0, 1))
					deadline.Set(time.Now().Add(time.Hour))
				}
			}()
		}
		wg.Wait()
	})
}

func BenchmarkDeadline(b *testing.B) {
	b.Run("Set", func(b *testing.B) {
		d := New()
		t := time.Now().Add(time.Minute)
		for i := 0; i < b.N; i++ {
			d.Set(t)
		}
	})
}
