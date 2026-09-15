// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build go1.25

package vnet

import (
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

type timestampedChunk struct {
	ts time.Time
	c  Chunk
}

func initDelayFilterTest(t *testing.T) (*DelayFilter, chan timestampedChunk) {
	t.Helper()
	nic := newMockNIC(t)
	delayFilter, err := NewDelayFilter(nic, WithDelay(0))
	if !assert.NoError(t, err, "should succeed") {
		return nil, nil
	}
	t.Cleanup(func() {
		assert.NoError(t, delayFilter.Close())
	})

	receiveCh := make(chan timestampedChunk)
	nic.mockOnInboundChunk = func(c Chunk) {
		select {
		case receiveCh <- timestampedChunk{ts: time.Now(), c: c}:
		case <-delayFilter.done:
		}
	}

	return delayFilter, receiveCh
}

func scheduleOnePacketAtATime(
	t *testing.T,
	delayFilter *DelayFilter,
	receiveCh chan timestampedChunk,
	delay time.Duration,
	nrPackets int,
) bool {
	t.Helper()
	delayFilter.SetDelay(delay)
	for i := range nrPackets {
		sent := time.Now()
		delayFilter.onInboundChunk(&chunkUDP{
			chunkIP:  chunkIP{timestamp: sent},
			userData: []byte{byte(i)},
		})

		select {
		case chunk := <-receiveCh:
			assert.Equal(t, i, int(chunk.c.UserData()[0]))
			assert.Equal(t, delay, chunk.ts.Sub(sent))
		case <-time.After(time.Second):
			assert.Fail(t, "expected to receive next chunk")

			return false
		}
	}

	return true
}

func scheduleManyPackets(
	t *testing.T,
	delayFilter *DelayFilter,
	receiveCh chan timestampedChunk,
	delay time.Duration,
	nrPackets int,
) bool {
	t.Helper()
	delayFilter.SetDelay(delay)
	sent := time.Now()

	for i := range nrPackets {
		delayFilter.onInboundChunk(&chunkUDP{
			chunkIP:  chunkIP{timestamp: sent},
			userData: []byte{byte(i)},
		})
	}

	for i := range nrPackets {
		select {
		case chunk := <-receiveCh:
			assert.Equal(t, i, int(chunk.c.UserData()[0]))
			assert.Equal(t, delay, chunk.ts.Sub(sent))
		case <-time.After(time.Second):
			assert.Fail(t, "expected to receive next chunk")

			return false
		}
	}

	return true
}

func TestDelayFilter(t *testing.T) {
	tests := []struct {
		name      string
		schedule  func(*testing.T, *DelayFilter, chan timestampedChunk, time.Duration, int) bool
		delays    []time.Duration
		nrPackets int
	}{
		{
			name:      "schedulesOnePacketAtATime",
			schedule:  scheduleOnePacketAtATime,
			delays:    []time.Duration{10 * time.Millisecond},
			nrPackets: 100,
		},
		{
			name:      "schedulesSubsequentManyPackets",
			schedule:  scheduleManyPackets,
			delays:    []time.Duration{10 * time.Millisecond},
			nrPackets: 100,
		},
		{
			name:      "scheduleIncreasingDelayOnePacketAtATime",
			schedule:  scheduleOnePacketAtATime,
			delays:    []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond},
			nrPackets: 10,
		},
		{
			name:      "scheduleDecreasingDelayOnePacketAtATime",
			schedule:  scheduleOnePacketAtATime,
			delays:    []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 10 * time.Millisecond},
			nrPackets: 10,
		},
		{
			name:      "scheduleIncreasingDelayManyPackets",
			schedule:  scheduleManyPackets,
			delays:    []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond},
			nrPackets: 100,
		},
		{
			name:      "scheduleDecreasingDelayManyPackets",
			schedule:  scheduleManyPackets,
			delays:    []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 10 * time.Millisecond},
			nrPackets: 100,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				delayFilter, receiveCh := initDelayFilterTest(t)
				if delayFilter == nil {
					return
				}

				for _, delay := range test.delays {
					if !test.schedule(t, delayFilter, receiveCh, delay, test.nrPackets) {
						return
					}
				}
			})
		})
	}
}

func TestDuplicationFilterDuplicates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		loggerFactory := logging.NewDefaultLoggerFactory()

		router, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		nic := make([]*dummyNIC, 2)
		ip := make([]*net.UDPAddr, 2)

		for i := range 2 {
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

		dupFilter, err := NewDuplicationFilter(
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

			return
		}

		select {
		case second := <-received:
			elapsed := time.Since(start)
			assert.Equal(t, 20*time.Millisecond, elapsed)
			assert.Equal(t, payload, second.UserData())
		case <-time.After(400 * time.Millisecond):
			assert.Fail(t, "expected duplicate chunk")
		}
	})
}

func TestDuplicationFilterZeroDelayNoLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		loggerFactory := logging.NewDefaultLoggerFactory()

		router, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		nic := make([]*dummyNIC, 2)
		ip := make([]*net.UDPAddr, 2)

		for i := range 2 {
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

		dupFilter, err := NewDuplicationFilter(
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

			return
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
	})
}

func TestDuplicationFilterCloseCancelsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		loggerFactory := logging.NewDefaultLoggerFactory()

		router, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		nic := make([]*dummyNIC, 2)
		ip := make([]*net.UDPAddr, 2)

		for i := range 2 {
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

		dupFilter, err := NewDuplicationFilter(
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

			return
		}

		assert.NoError(t, dupFilter.Close())

		select {
		case extra := <-received:
			assert.Failf(t, "duplicate delivered after close", "unexpected chunk: %s", extra.String())
		case <-time.After(300 * time.Millisecond):
		}
	})
}

func TestDuplicationFilterBucketCoalescing(t *testing.T) { //nolint:cyclop
	synctest.Test(t, func(t *testing.T) {
		loggerFactory := logging.NewDefaultLoggerFactory()

		router, err := NewRouter(&RouterConfig{
			CIDR:          "1.2.3.0/24",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		nic := make([]*dummyNIC, 2)
		ip := make([]*net.UDPAddr, 2)

		for i := range 2 {
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

		dupFilter, err := NewDuplicationFilter(
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

		for i := range size {
			chunk := newChunkUDP(ip[0], ip[1]) //nolint:gosec // ip slice length is 2
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
				assert.Failf(t, "timeout waiting for arrivals", "expected %d arrivals, got %d", 2*size, done)

				return
			}
		}

		var firstDup, lastDup time.Time
		for i := range size {
			pair := arrivals[i]
			if !assert.Len(t, pair, 2, "expected primary and duplicate for index %d", i) {
				continue
			}
			primary, duplicate := pair[0].t, pair[1].t

			dupDelay := duplicate.Sub(primary)
			// The 500us delay rounds up to the next 1ms bucket in fake time.
			assert.Equal(t, time.Millisecond, dupDelay)

			if firstDup.IsZero() || duplicate.Before(firstDup) {
				firstDup = duplicate
			}
			if lastDup.IsZero() || duplicate.After(lastDup) {
				lastDup = duplicate
			}
		}

		coalesceSpan := lastDup.Sub(firstDup)
		assert.Zero(t, coalesceSpan)
	})
}
