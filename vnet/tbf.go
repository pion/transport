package vnet

import (
	"math"
	"sync"
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
	currentTokensInBucket int64
	c                     chan Chunk

	lastAdd time.Time

	mutex    sync.Mutex
	rate     int64
	maxBurst int64
}

// TBFOption is the option type to configure a TokenBucketFilter
type TBFOption func(*TokenBucketFilter) TBFOption

// TBFRate sets the bitrate of a TokenBucketFilter
func TBFRate(rate int64) TBFOption {
	return func(t *TokenBucketFilter) TBFOption {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		previous := t.rate
		t.rate = rate
		return TBFRate(previous)
	}
}

// TBFMaxBurst sets the bucket size of the token bucket filter. This is the
// maximum size that can instantly leave the filter, if the bucket is full.
func TBFMaxBurst(size int64) TBFOption {
	return func(t *TokenBucketFilter) TBFOption {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		previous := t.maxBurst
		t.maxBurst = size
		return TBFMaxBurst(previous)
	}
}

// Set updates a setting on the token bucket filter
func (t *TokenBucketFilter) Set(opts ...TBFOption) (previous TBFOption) {
	for _, opt := range opts {
		previous = opt(t)
	}
	return previous
}

// NewTokenBucketFilter creates and starts a new TokenBucketFilter
func NewTokenBucketFilter(n NIC, opts ...TBFOption) (*TokenBucketFilter, error) {
	tbf := &TokenBucketFilter{
		NIC: n,
		c:   make(chan Chunk),

		lastAdd: time.Now(),

		rate:                  1 * MBit,
		maxBurst:              20 * KBit,
		currentTokensInBucket: 20 * KBit,
	}
	tbf.Set(opts...)
	return tbf, nil
}

func (t *TokenBucketFilter) addTokens() {
	now := time.Now()
	if !now.After(t.lastAdd) {
		return
	}
	d := now.Sub(t.lastAdd)
	us := d.Microseconds()
	add := float64(us) * (float64(t.rate) / float64(1e+6)) / 8
	t.currentTokensInBucket = int64(math.Min(float64(t.maxBurst), float64(t.currentTokensInBucket)+add))
	t.lastAdd = now
}

func (t *TokenBucketFilter) onInboundChunk(c Chunk) {
	t.addTokens()
	tokens := int64(len(c.UserData()))
	if t.currentTokensInBucket > tokens {
		t.NIC.onInboundChunk(c)
		t.currentTokensInBucket -= tokens
	}
}
