// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"math/rand"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestDuplicationFilterDuplicates(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: loggerFactory,
	})
	assert.NoError(t, err)

	nic := make([]*dummyNIC, 2)
	ip := make([]*net.UDPAddr, 2)

	for i := 0; i < 2; i++ {
		anet, netErr := NewNet(&NetConfig{})
		assert.NoError(t, netErr)

		nic[i] = &dummyNIC{Net: anet}
		assert.NoError(t, router.AddNet(nic[i]))

		eth0, errInterface := nic[i].getInterface("eth0")
		assert.NoError(t, errInterface)

		addrs, errAddrs := eth0.Addrs()
		assert.NoError(t, errAddrs)
		assert.Equal(t, 1, len(addrs))

		ip[i] = &net.UDPAddr{ //nolint:forcetypeassert
			IP:   addrs[0].(*net.IPNet).IP,
			Port: 10000 + i,
		}
	}

	dupFilter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(20*time.Millisecond, 20*time.Millisecond),
		WithDuplicationSeed(1),
	)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, dupFilter.Close())
	}()

	router.AddChunkFilter(dupFilter.ChunkFilter())

	received := make(chan Chunk, 4)
	nic[1].onInboundChunkHandler = func(c Chunk) {
		received <- c
	}

	assert.NoError(t, router.Start())
	defer func() {
		assert.NoError(t, router.Stop())
	}()

	chunk := newChunkUDP(ip[0], ip[1])
	payload := []byte{0x42}
	chunk.userData = make([]byte, len(payload))
	copy(chunk.userData, payload)

	start := time.Now()
	router.push(chunk)

	select {
	case first := <-received:
		assert.Equal(t, payload, first.UserData())
	case <-time.After(200 * time.Millisecond):
		assert.Fail(t, "expected primary chunk")
	}

	select {
	case second := <-received:
		elapsed := time.Since(start)
		assert.GreaterOrEqual(t, elapsed, 18*time.Millisecond)
		assert.Equal(t, payload, second.UserData())
	case <-time.After(400 * time.Millisecond):
		assert.Fail(t, "expected duplicate chunk")
	}
}

func TestDuplicationFilterConfigValidation(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationProbability(-0.1))
	assert.ErrorIs(t, err, errInvalidDuplicationProbability)
	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationProbability(1.1))
	assert.ErrorIs(t, err, errInvalidDuplicationProbability)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationBurstProbability(1.5))
	assert.ErrorIs(t, err, errInvalidDuplicationBurstProbability)
	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationBurstProbability(-0.1))
	assert.ErrorIs(t, err, errInvalidDuplicationBurstProbability)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationExtraDelay(time.Millisecond, -time.Millisecond))
	assert.ErrorIs(t, err, errInvalidDuplicationDelayRange)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationExtraDelay(-time.Millisecond, time.Millisecond))
	assert.ErrorIs(t, err, errInvalidDuplicationDelayRange)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationExtraDelay(time.Millisecond, time.Millisecond-1))
	assert.ErrorIs(t, err, errInvalidDuplicationDelayRange)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationBurstDuration(-1))
	assert.ErrorIs(t, err, errInvalidDuplicationBurstDuration)

	_, err = NewDuplicationFilterWithOptions(router, WithDuplicationBurstMultiplier(0.5))
	assert.ErrorIs(t, err, errInvalidDuplicationBurstMultiplier)

	_, err = NewDuplicationFilterWithOptions(nil, WithDuplicationProbability(0.5))
	assert.ErrorIs(t, err, errInvalidDuplicationRouter)
}

func TestValidateDuplicationConfig_Errors(t *testing.T) {
	tests := []struct {
		name     string
		override duplicationConfig
		err      error
	}{
		{
			name:     "invalid prob < 0",
			override: duplicationConfig{prob: -0.01},
			err:      errInvalidDuplicationProbability,
		},
		{
			name:     "invalid prob > 1",
			override: duplicationConfig{prob: 1.01},
			err:      errInvalidDuplicationProbability,
		},
		{
			name:     "invalid burstStartProb < 0",
			override: duplicationConfig{burstStartProb: -0.1},
			err:      errInvalidDuplicationBurstProbability,
		},
		{
			name:     "invalid burstStartProb > 1",
			override: duplicationConfig{burstStartProb: 1.1},
			err:      errInvalidDuplicationBurstProbability,
		},
		{
			name:     "invalid burstMultiplier < 1",
			override: duplicationConfig{burstMultiplier: 0.5},
			err:      errInvalidDuplicationBurstMultiplier,
		},
		{
			name:     "invalid burstDuration < 0",
			override: duplicationConfig{burstDuration: -1},
			err:      errInvalidDuplicationBurstDuration,
		},
		{
			name:     "invalid delay min < 0",
			override: duplicationConfig{minExtraDelay: -time.Nanosecond, maxExtraDelay: 0},
			err:      errInvalidDuplicationDelayRange,
		},
		{
			name:     "invalid delay max < 0",
			override: duplicationConfig{minExtraDelay: 0, maxExtraDelay: -time.Nanosecond},
			err:      errInvalidDuplicationDelayRange,
		},
		{
			name:     "invalid delay max < min",
			override: duplicationConfig{minExtraDelay: time.Millisecond, maxExtraDelay: time.Millisecond - 1},
			err:      errInvalidDuplicationDelayRange,
		},
	}

	for _, tc := range tests {
		cfg := duplicationConfig{
			prob:            0,
			burstStartProb:  0,
			burstDuration:   0,
			burstMultiplier: 1,
			minExtraDelay:   0,
			maxExtraDelay:   0,
		}

		if tc.override.prob != 0 {
			cfg.prob = tc.override.prob
		}
		if tc.override.burstStartProb != 0 {
			cfg.burstStartProb = tc.override.burstStartProb
		}
		if tc.override.burstDuration != 0 {
			cfg.burstDuration = tc.override.burstDuration
		}
		if tc.override.burstMultiplier != 0 {
			cfg.burstMultiplier = tc.override.burstMultiplier
		}
		if tc.override.minExtraDelay != 0 || tc.override.maxExtraDelay != 0 {
			cfg.minExtraDelay = tc.override.minExtraDelay
			cfg.maxExtraDelay = tc.override.maxExtraDelay
		}

		err := validateDuplicationConfig(&cfg)
		assert.ErrorIs(t, err, tc.err, tc.name)
	}
}

func TestDuplicationImmediateOption(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationImmediate(),
		WithDuplicationSeed(123),
	)
	assert.NoError(t, err)

	delay, dup := filter.shouldDuplicate()
	assert.True(t, dup)
	assert.Equal(t, time.Duration(0), delay)
}

func TestDuplicationFilterClosedSkipsDuplication(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(1*time.Millisecond, 1*time.Millisecond),
		WithDuplicationSeed(9),
	)
	assert.NoError(t, err)

	assert.NoError(t, filter.Close())

	_, dup := filter.shouldDuplicate()
	assert.False(t, dup)
}

func TestDuplicationDelayRangeInclusive(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(0, 1*time.Nanosecond),
		WithDuplicationSeed(42),
	)
	assert.NoError(t, err)

	sawZero := false
	sawMax := false
	deadline := time.Now().Add(2 * time.Second)
	for (!sawZero || !sawMax) && time.Now().Before(deadline) {
		d, ok := filter.shouldDuplicate()
		if !ok {
			continue
		}
		if d == 0 {
			sawZero = true
		}
		if d == 1*time.Nanosecond {
			sawMax = true
		}
	}

	assert.True(t, sawZero, "expected to observe minimum delay")
	assert.True(t, sawMax, "expected to observe maximum delay inclusively")
}

func TestDuplicationFilterZeroDelayNoLoop(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: loggerFactory,
	})
	assert.NoError(t, err)

	nic := make([]*dummyNIC, 2)
	ip := make([]*net.UDPAddr, 2)

	for i := 0; i < 2; i++ {
		anet, netErr := NewNet(&NetConfig{})
		assert.NoError(t, netErr)

		nic[i] = &dummyNIC{Net: anet}
		assert.NoError(t, router.AddNet(nic[i]))

		eth0, errInterface := nic[i].getInterface("eth0")
		assert.NoError(t, errInterface)

		addrs, errAddrs := eth0.Addrs()
		assert.NoError(t, errAddrs)
		assert.Equal(t, 1, len(addrs))

		ip[i] = &net.UDPAddr{ //nolint:forcetypeassert
			IP:   addrs[0].(*net.IPNet).IP,
			Port: 11000 + i,
		}
	}

	dupFilter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(0, 0),
		WithDuplicationSeed(7),
	)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, dupFilter.Close())
	}()

	router.AddChunkFilter(dupFilter.ChunkFilter())

	received := make(chan Chunk, 8)
	nic[1].onInboundChunkHandler = func(c Chunk) {
		received <- c
	}

	assert.NoError(t, router.Start())
	defer func() {
		assert.NoError(t, router.Stop())
	}()

	chunk := newChunkUDP(ip[0], ip[1])
	payload := []byte{0x99}
	chunk.userData = make([]byte, len(payload))
	copy(chunk.userData, payload)

	router.push(chunk)

	select {
	case first := <-received:
		assert.Equal(t, payload, first.UserData())
	case <-time.After(200 * time.Millisecond):
		assert.Fail(t, "expected primary chunk")
	}

	select {
	case second := <-received:
		assert.Equal(t, payload, second.UserData())
	case <-time.After(200 * time.Millisecond):
		assert.Fail(t, "expected duplicate chunk")
	}

	select {
	case extra := <-received:
		assert.Failf(t, "duplicate loop detected", "unexpected chunk: %s", extra.String())
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDuplicationFilterBurstMultiplier(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	baseOpts := []DuplicationOption{
		WithDuplicationProbability(0.2),
		WithDuplicationBurstProbability(1.0),
		WithDuplicationBurstDuration(50 * time.Millisecond),
		WithDuplicationExtraDelay(0, 0),
	}

	baseline, err := NewDuplicationFilterWithOptions(router, append(baseOpts, WithDuplicationBurstMultiplier(1))...)
	assert.NoError(t, err)
	boosted, err := NewDuplicationFilterWithOptions(router, append(baseOpts, WithDuplicationBurstMultiplier(5))...)
	assert.NoError(t, err)

	now := time.Now()
	baseline.now = func() time.Time { return now }
	boosted.now = baseline.now
	baseline.burstEnd = now.Add(25 * time.Millisecond)
	boosted.burstEnd = baseline.burstEnd

	baseline.rng = rand.New(rand.NewSource(1)) //nolint:gosec // weak rand is intended
	boosted.rng = rand.New(rand.NewSource(1))  //nolint:gosec // weak rand is intended

	_, baselineDup := baseline.shouldDuplicate()
	_, boostedDup := boosted.shouldDuplicate()

	assert.False(t, baselineDup, "baseline probability should fail for value above 0.2")
	assert.True(t, boostedDup, "burst multiplier should allow duplication")
}

func TestDuplicationFilterCloseCancelsPending(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: loggerFactory,
	})
	assert.NoError(t, err)

	nic := make([]*dummyNIC, 2)
	ip := make([]*net.UDPAddr, 2)

	for i := 0; i < 2; i++ {
		anet, netErr := NewNet(&NetConfig{})
		assert.NoError(t, netErr)

		nic[i] = &dummyNIC{Net: anet}
		assert.NoError(t, router.AddNet(nic[i]))

		eth0, errInterface := nic[i].getInterface("eth0")
		assert.NoError(t, errInterface)

		addrs, errAddrs := eth0.Addrs()
		assert.NoError(t, errAddrs)
		assert.Equal(t, 1, len(addrs))

		ip[i] = &net.UDPAddr{ //nolint:forcetypeassert
			IP:   addrs[0].(*net.IPNet).IP,
			Port: 12000 + i,
		}
	}

	dupFilter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(100*time.Millisecond, 100*time.Millisecond),
		WithDuplicationSeed(3),
	)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, dupFilter.Close())
	}()

	router.AddChunkFilter(dupFilter.ChunkFilter())

	received := make(chan Chunk, 4)
	nic[1].onInboundChunkHandler = func(c Chunk) {
		received <- c
	}

	assert.NoError(t, router.Start())
	defer func() {
		assert.NoError(t, router.Stop())
	}()

	chunk := newChunkUDP(ip[0], ip[1])
	chunk.userData = []byte{0x01}
	router.push(chunk)

	select {
	case <-received:
		// primary chunk delivered
	case <-time.After(200 * time.Millisecond):
		assert.Fail(t, "expected primary chunk before close")
	}

	assert.NoError(t, dupFilter.Close())

	select {
	case extra := <-received:
		assert.Failf(t, "duplicate delivered after close", "unexpected chunk: %s", extra.String())
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDuplicationFilterProbabilityLongRun(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(0.3),
		WithDuplicationExtraDelay(0, 0),
		WithDuplicationSeed(2023),
	)
	assert.NoError(t, err)

	const iterations = 50000
	dupCount := 0
	for i := 0; i < iterations; i++ {
		_, dup := filter.shouldDuplicate()
		if dup {
			dupCount++
		}
	}

	ratio := float64(dupCount) / float64(iterations)
	assert.InDelta(t, 0.3, ratio, 0.02)
}

func TestDuplicationFilterBucketCoalescing(t *testing.T) { //nolint:cyclop
	loggerFactory := logging.NewDefaultLoggerFactory()

	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: loggerFactory,
	})
	assert.NoError(t, err)

	nic := make([]*dummyNIC, 2)
	ip := make([]*net.UDPAddr, 2)

	for i := 0; i < 2; i++ {
		anet, netErr := NewNet(&NetConfig{})
		assert.NoError(t, netErr)

		nic[i] = &dummyNIC{Net: anet}
		assert.NoError(t, router.AddNet(nic[i]))

		eth0, errInterface := nic[i].getInterface("eth0")
		assert.NoError(t, errInterface)

		addrs, errAddrs := eth0.Addrs()
		assert.NoError(t, errAddrs)
		assert.Equal(t, 1, len(addrs))

		ip[i] = &net.UDPAddr{ //nolint:forcetypeassert
			IP:   addrs[0].(*net.IPNet).IP,
			Port: 13000 + i,
		}
	}

	dupFilter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(500*time.Microsecond, 500*time.Microsecond),
		WithDuplicationSeed(11),
	)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, dupFilter.Close())
	}()

	router.AddChunkFilter(dupFilter.ChunkFilter())

	type arrival struct{ t time.Time }
	const size = 40
	arrivals := make([][]arrival, size)

	received := make(chan Chunk, 2*size)
	nic[1].onInboundChunkHandler = func(c Chunk) {
		received <- c
	}

	assert.NoError(t, router.Start())
	defer func() {
		assert.NoError(t, router.Stop())
	}()

	for i := 0; i < size; i++ {
		chunk := newChunkUDP(ip[0], ip[1])
		payload := []byte{byte(i)}
		chunk.userData = make([]byte, len(payload))
		copy(chunk.userData, payload)
		router.push(chunk)
	}

	deadline := time.After(2 * time.Second)
	for done := 0; done < 2*size; {
		select {
		case c := <-received:
			if data := c.UserData(); len(data) == 1 {
				idx := int(data[0])
				arrivals[idx] = append(arrivals[idx], arrival{t: time.Now()})
				done++
			}
		case <-deadline:
			assert.Fail(t, "timeout waiting for %d arrivals, got %d", 2*size, done)
		}
	}

	var firstDup, lastDup time.Time
	for i := 0; i < size; i++ {
		pair := arrivals[i]
		assert.Len(t, pair, 2, "expected primary and duplicate for index %d", i)
		primary, duplicate := pair[0].t, pair[1].t

		dupDelay := duplicate.Sub(primary)
		minDelay := 100 * time.Microsecond
		if runtime.GOOS == "windows" {
			// Windows timers can have low resolution.
			minDelay = 0
		}
		assert.GreaterOrEqual(t, dupDelay, minDelay)
		assert.LessOrEqual(t, dupDelay, 5*time.Millisecond)

		if firstDup.IsZero() || duplicate.Before(firstDup) {
			firstDup = duplicate
		}
		if lastDup.IsZero() || duplicate.After(lastDup) {
			lastDup = duplicate
		}
	}

	coalesceSpan := lastDup.Sub(firstDup)
	assert.LessOrEqual(t, coalesceSpan, 6*time.Millisecond)
}

func TestDuplicationFilterBurstStartSetsWindow(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(0.0),
		WithDuplicationBurstProbability(1.0),
		WithDuplicationBurstDuration(10*time.Millisecond),
		WithDuplicationExtraDelay(0, 0),
		WithDuplicationSeed(1234),
	)
	assert.NoError(t, err)

	fixedNow := time.Now()
	filter.now = func() time.Time { return fixedNow }

	assert.True(t, filter.burstEnd.IsZero())

	_, _ = filter.shouldDuplicate()

	expectedEnd := fixedNow.Add(10 * time.Millisecond)
	assert.WithinDuration(t, expectedEnd, filter.burstEnd, time.Microsecond)
}

func TestDuplicationFilterOnBucketFiredClosedCleans(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(0.0),
		WithDuplicationExtraDelay(0, 0),
		WithDuplicationSeed(1),
	)
	assert.NoError(t, err)

	key := time.Now().Add(1 * time.Minute).UnixNano()
	tm := time.NewTimer(10 * time.Minute)
	defer tm.Stop()

	filter.mu.Lock()
	b := &dupBucket{timer: tm}
	filter.buckets[key] = b
	filter.timers[tm] = struct{}{}
	filter.closed = true
	filter.mu.Unlock()

	filter.onBucketFired(key)

	filter.mu.Lock()
	_, bucketExists := filter.buckets[key]
	_, timerExists := filter.timers[tm]
	filter.mu.Unlock()

	assert.False(t, bucketExists, "bucket should be removed when closed")
	assert.False(t, timerExists, "timer should be removed when closed")
}

func TestDuplicationFilterScheduleDuplicateClosedNoop(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		WithDuplicationExtraDelay(1*time.Millisecond, 1*time.Millisecond),
		WithDuplicationSeed(5),
	)
	assert.NoError(t, err)

	filter.mu.Lock()
	filter.closed = true
	initialTimers := len(filter.timers)
	initialBuckets := len(filter.buckets)
	filter.mu.Unlock()

	dup := newChunkUDP(&net.UDPAddr{}, &net.UDPAddr{})
	filter.scheduleDuplicate(dup, 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	filter.mu.Lock()
	defer filter.mu.Unlock()
	assert.Equal(t, initialTimers, len(filter.timers))
	assert.Equal(t, initialBuckets, len(filter.buckets))
}

func TestNewDuplicationFilter_NilOptionIgnored(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(1.0),
		// should be ignored.
		nil,
		WithDuplicationImmediate(),
		WithDuplicationSeed(12345),
	)
	assert.NoError(t, err)

	defer func() { _ = filter.Close() }()

	delay, dup := filter.shouldDuplicate()
	assert.True(t, dup)
	assert.Equal(t, time.Duration(0), delay)
}

func TestNewDuplicationFilter_ValidateCalledOnOptions(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	invalidNoReturn := func(cfg *duplicationConfig) error {
		cfg.burstMultiplier = 0

		return nil
	}

	_, err = NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(0.5),
		DuplicationOption(invalidNoReturn),
	)
	assert.ErrorIs(t, err, errInvalidDuplicationBurstMultiplier)
}

func TestDuplicationFilterBurstImmediateAppliesMultiplier(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	// choose a seed and precompute the first two Float64 draws.
	// the first draw will be consumed by the burst-start check,
	// the second draw will be used for the duplication probability check.
	const seed int64 = 99
	probe := rand.New(rand.NewSource(seed)) //nolint:gosec // weak rand is intended
	_ = probe.Float64()                     // consumed by burst-start path
	secondDraw := probe.Float64()           // will be used for duplication probability

	// pick base probability just below the second draw so that without multiplier
	// the duplication would NOT occur (rng >= baseProb).
	const eps = 1e-9
	baseProb := secondDraw - eps
	if baseProb < 0 {
		baseProb = 0
	}

	// multiplier 2x ensures boosted probability > secondDraw (or clamps to 1),
	// so duplication should occur on the same call where the burst starts.
	const mult = 2.0

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(baseProb),
		WithDuplicationBurstProbability(1.0),
		WithDuplicationBurstDuration(10*time.Millisecond),
		WithDuplicationBurstMultiplier(mult),
		WithDuplicationExtraDelay(0, 0),
		WithDuplicationSeed(seed),
	)
	assert.NoError(t, err)

	fixedNow := time.Now()
	filter.now = func() time.Time { return fixedNow }
	// Ensure we are outside any burst window before the call
	filter.burstEnd = time.Time{}

	// On this single call:
	// 1) burst will start (burstStartProb=1)
	// 2) multiplier must apply immediately to this packet
	// 3) duplication should succeed deterministically
	_, dup := filter.shouldDuplicate()
	assert.True(t, dup, "expected immediate application of burst multiplier to current packet")
}

func TestDuplicationFilterBurstBoundaryEqualityNotInBurst(t *testing.T) {
	router, err := NewRouter(&RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	// use a known seed and compute the first Float64 draw which will be used
	// for the duplication probability check (no burst-start draw will happen
	// since we are exactly at the boundary. neither After nor Before).
	const seed int64 = 123
	probe := rand.New(rand.NewSource(seed)) //nolint:gosec // weak rand is intended
	firstDraw := probe.Float64()

	const eps = 1e-9
	baseProb := firstDraw - eps
	if baseProb < 0 {
		baseProb = 0
	}

	filter, err := NewDuplicationFilterWithOptions(
		router,
		WithDuplicationProbability(baseProb),
		WithDuplicationBurstProbability(1.0),
		WithDuplicationBurstDuration(10*time.Millisecond),
		WithDuplicationBurstMultiplier(10.0),
		WithDuplicationExtraDelay(0, 0),
		WithDuplicationSeed(seed),
	)
	assert.NoError(t, err)

	fixedNow := time.Now()
	filter.now = func() time.Time { return fixedNow }
	// set burstEnd exactly to now: equality should be treated as out-of-burst
	filter.burstEnd = fixedNow

	_, dup := filter.shouldDuplicate()
	assert.False(t, dup, "equality boundary must not apply burst multiplier")
}
