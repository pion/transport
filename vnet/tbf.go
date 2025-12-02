// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import "sync"

const (
	// Bit is a single bit.
	Bit = 1
	// KBit is a kilobit.
	KBit = 1000 * Bit
	// MBit is a Megabit.
	MBit = 1000 * KBit
)

// TokenBucketFilter implements a token bucket rate limit algorithm.
//
// Deprecated: TokenBucketFilter is now a wrapper around Queue with a TBF
// discipline. Use Queue directly.
type TokenBucketFilter struct {
	NIC
	queue *Queue
	tbf   *TBFQueue

	lock    sync.Mutex
	maxSize int64
	rate    int
	burst   int
}

// TBFOption is the option type to configure a TokenBucketFilter.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
type TBFOption func(*TokenBucketFilter) TBFOption

// TBFQueueSizeInBytes sets the max number of bytes waiting in the queue. Can
// only be set in constructor before using the TBF.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
func TBFQueueSizeInBytes(bytes int) TBFOption {
	return func(t *TokenBucketFilter) TBFOption {
		t.lock.Lock()
		defer t.lock.Unlock()
		prev := t.maxSize
		t.maxSize = int64(bytes)
		t.tbf.SetSize(t.maxSize)

		return TBFQueueSizeInBytes(int(prev))
	}
}

// TBFRate sets the bit rate of a TokenBucketFilter.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
func TBFRate(rate int) TBFOption {
	return func(t *TokenBucketFilter) TBFOption {
		t.lock.Lock()
		defer t.lock.Unlock()
		prev := t.rate
		t.rate = rate
		t.tbf.SetRate(t.rate)

		return TBFRate(prev)
	}
}

// TBFMaxBurst sets the bucket size of the token bucket filter. This is the
// maximum size that can instantly leave the filter, if the bucket is full.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
func TBFMaxBurst(size int) TBFOption {
	return func(t *TokenBucketFilter) TBFOption {
		t.lock.Lock()
		defer t.lock.Unlock()
		prev := t.burst
		t.burst = size
		t.tbf.SetBurst(t.burst)

		return TBFMaxBurst(prev)
	}
}

// Set updates a setting on the token bucket filter.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
func (t *TokenBucketFilter) Set(opts ...TBFOption) (previous TBFOption) {
	for _, opt := range opts {
		previous = opt(t)
	}

	return previous
}

// NewTokenBucketFilter creates and starts a new TokenBucketFilter.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
func NewTokenBucketFilter(n NIC, opts ...TBFOption) (*TokenBucketFilter, error) {
	tbfQueue := NewTBFQueue(1*MBit, 8*KBit, int64(50_000))
	q, err := NewQueue(n, tbfQueue)
	if err != nil {
		return nil, err
	}
	tbf := &TokenBucketFilter{
		NIC:   q.NIC,
		tbf:   tbfQueue,
		queue: q,
	}
	tbf.Set(opts...)

	return tbf, nil
}

func (t *TokenBucketFilter) onInboundChunk(c Chunk) {
	t.queue.onInboundChunk(c)
}

// Close closes and stops the token bucket filter queue.
//
// Deprecated: TokenBucketFilter is deprecated, use Queue instead.
func (t *TokenBucketFilter) Close() error {
	return t.queue.Close()
}
