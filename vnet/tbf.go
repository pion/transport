package vnet

import (
	"context"
	"time"
)

const (
	// Bit is a single bit
	Bit = 1
	// KBit is a kilobit
	KBit = 1000 * Bit
	// MBit is a Megabit
	MBit = 1000 * KBit
)

// TokenBucketFilter implements a token bucket rate limit algorithm.
type TokenBucketFilter struct {
	NIC
	currentTokensInBucket int
	c                     chan Chunk

	rate     int
	maxBurst int
}

// TBFOption is the option type to configure a TokenBucketFilter
type TBFOption func(*TokenBucketFilter) error

// TBFRate sets the bitrate of a TokenBucketFilter
func TBFRate(rate int) TBFOption {
	return func(t *TokenBucketFilter) error {
		t.rate = rate
		return nil
	}
}

// TBFMaxBurst sets the bucket size of the token bucket filter. This is the
// maximum size that can instantly leave the filter, if the bucket is full.
func TBFMaxBurst(size int) TBFOption {
	return func(t *TokenBucketFilter) error {
		t.maxBurst = size
		return nil
	}
}

// NewTokenBucketFilter creates and starts a new TokenBucketFilter
func NewTokenBucketFilter(ctx context.Context, n NIC, opts ...TBFOption) (*TokenBucketFilter, error) {
	tbf := &TokenBucketFilter{
		NIC: n,
		c:   make(chan Chunk),

		rate:     1 * MBit,
		maxBurst: 2 * KBit,
	}
	for _, opt := range opts {
		err := opt(tbf)
		if err != nil {
			return nil, err
		}
	}
	go tbf.run(ctx)
	return tbf, nil
}

func (t *TokenBucketFilter) onInboundChunk(c Chunk) {
	t.c <- c
}

func (t *TokenBucketFilter) run(ctx context.Context) {
	// (bitrate * S) / 1000 converted to bytes (divide by 8)
	// S is the update interval in milliseconds
	add := (t.rate / 1000) / 8
	ticker := time.NewTicker(1 * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			if t.currentTokensInBucket < t.maxBurst {
				t.currentTokensInBucket += add
			}
		case chunk := <-t.c:
			tokens := len(chunk.UserData())
			if t.currentTokensInBucket > tokens {
				t.NIC.onInboundChunk(chunk)
				t.currentTokensInBucket -= tokens
			}
		}
	}
}
