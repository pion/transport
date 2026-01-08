// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"testing"

	"github.com/pion/transport/v4"
	"github.com/stretchr/testify/assert"
)

type mockNIC struct {
	mockGetInterface   func(ifName string) (*transport.Interface, error)
	mockOnInboundChunk func(c Chunk)
	mockGetStaticIPs   func() []net.IP
	mockSetRouter      func(r *Router) error
}

func (n *mockNIC) getInterface(ifName string) (*transport.Interface, error) {
	return n.mockGetInterface(ifName)
}

func (n *mockNIC) onInboundChunk(c Chunk) {
	n.mockOnInboundChunk(c)
}

func (n *mockNIC) getStaticIPs() []net.IP {
	return n.mockGetStaticIPs()
}

func (n *mockNIC) setRouter(r *Router) error {
	return n.mockSetRouter(r)
}

func newMockNIC(t *testing.T) *mockNIC {
	t.Helper()

	return &mockNIC{
		mockGetInterface: func(string) (*transport.Interface, error) {
			assert.Fail(t, "unexpected call to mockGetInterface")

			return nil, nil
		},
		mockOnInboundChunk: func(Chunk) {
			assert.Fail(t, "unexpected call to mockOnInboundChunk")
		},
		mockGetStaticIPs: func() []net.IP {
			assert.Fail(t, "unexpected call to mockGetStaticIPs")

			return nil
		},
		mockSetRouter: func(*Router) error {
			assert.Fail(t, "unexpected call to mockSetRouter")

			return nil
		},
	}
}

func TestLossFilterFullLoss(t *testing.T) {
	mnic := newMockNIC(t)

	lossFilter, err := NewLossFilter(mnic, 100)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	lossFilter.onInboundChunk(&chunkUDP{})
}

func TestLossFilterNoLoss(t *testing.T) {
	mnic := newMockNIC(t)

	lossFilter, err := NewLossFilter(mnic, 0)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	packets := 100
	received := 0
	mnic.mockOnInboundChunk = func(Chunk) {
		received++
	}

	for i := 0; i < packets; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, packets, received)
}

func TestLossFilterSomeLoss(t *testing.T) {
	mnic := newMockNIC(t)

	lossFilter, err := NewLossFilter(mnic, 50)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	packets := 1000
	received := 0
	mnic.mockOnInboundChunk = func(Chunk) {
		received++
	}

	for i := 0; i < packets; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	// One of the following could technically fail, but very unlikely
	assert.Less(t, 0, received)
	assert.Greater(t, packets, received)
}

func TestLossFilterLossRateChangeRandomShuffleHandler(t *testing.T) {
	mnic := newMockNIC(t)

	lossHandler := newShuffleLossHandle(10, 100, newRNG(nil))

	lossFilter, err := NewLossFilter(mnic, 0, WithLossHandler(lossHandler))
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	packets := 100
	received := 0
	mnic.mockOnInboundChunk = func(Chunk) {
		received++
	}

	for i := 0; i < packets; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, 90, received)

	err = lossFilter.SetLossRate(50, true)
	if !assert.NoError(t, err, "should succeed") {
		return
	}
	received = 0
	for i := 0; i < packets; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, 50, received)

	err = lossFilter.SetLossRate(99, true)
	if !assert.NoError(t, err, "should succeed") {
		return
	}
	received = 0
	for i := 0; i < packets; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, 1, received)
}

func TestLossFilterImmediateLossRateChangeRandomShuffleHandler(t *testing.T) {
	mnic := newMockNIC(t)

	lossHandler := newShuffleLossHandle(10, 100, newRNG(nil))

	lossFilter, err := NewLossFilter(mnic, 0, WithLossHandler(lossHandler))
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	packets := 100
	received := 0
	mnic.mockOnInboundChunk = func(Chunk) {
		received++
	}

	// send 50 dummy packets to partially fill shuffle block
	for i := 0; i < 50; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	// should trigger an immediate shuffle that sets the loss rate to 50%
	err = lossFilter.SetLossRate(50, true)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	received = 0
	for i := 0; i < packets; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, 50, received)
}

func TestLossFilterNonImmediateLossRateChangeRandomShuffleHandler(t *testing.T) {
	mnic := newMockNIC(t)

	lossHandler := newShuffleLossHandle(10, 100, newRNG(nil))

	lossFilter, err := NewLossFilter(mnic, 0, WithLossHandler(lossHandler))
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	received := 0
	mnic.mockOnInboundChunk = func(Chunk) {
		received++
	}

	// send 50 dummy packets to partially fill shuffle block
	for i := 0; i < 50; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	_ = lossFilter.SetLossRate(100, false)

	// the loss rate should not be changed until the shuffle block is full
	for i := 0; i < 50; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, 90, received)

	received = 0

	// the new loss rate should be applied to this block
	for i := 0; i < 100; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, 0, received)
}

func TestLossFilterOptionsPattern(t *testing.T) {
	t.Run("WithLossHandler option", func(t *testing.T) {
		mnic := newMockNIC(t)

		customHandler := newShuffleLossHandle(10, 100, newRNG(nil))

		// Using options pattern
		lossFilter, err := NewLossFilter(mnic, 50, WithLossHandler(customHandler))
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		// Test that the custom handler is used
		packets := 100
		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		for i := 0; i < packets; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		// Should use the custom handler's behavior (10% loss from handler creation)
		// not the 50% from NewLossFilterWithOptions chance parameter
		assert.Equal(t, 90, received)
	})

	t.Run("WithShuffleLossHandler option", func(t *testing.T) {
		mnic := newMockNIC(t)

		// Using options pattern with shuffle handler
		lossFilter, err := NewLossFilter(mnic, 25, WithShuffleLossHandler(100))
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		packets := 100
		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		for i := 0; i < packets; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		// Should use shuffle handler behavior with 25% loss rate
		assert.Equal(t, 75, received)
	})

	t.Run("Backward compatibility - no options", func(t *testing.T) {
		mnic := newMockNIC(t)

		// Old API should still work
		lossFilter, err := NewLossFilter(mnic, 20)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		packets := 1000
		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		for i := 0; i < packets; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		// Should work as before with random loss handler
		assert.Less(t, 0, received)
		assert.Greater(t, packets, received)
	})
}

func testShuffleRounding(t *testing.T, chance int, blockSize int, expectedReceived int) {
	t.Helper()
	mnic := newMockNIC(t)
	lossFilter, err := NewLossFilter(mnic, chance, WithShuffleLossHandler(blockSize))
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	received := 0
	mnic.mockOnInboundChunk = func(Chunk) {
		received++
	}

	for i := 0; i < 100; i++ {
		lossFilter.onInboundChunk(&chunkUDP{})
	}

	assert.Equal(t, expectedReceived, received)
}

func TestLossFilterShuffleRounding(t *testing.T) {
	t.Run("1% with blockSize 10", func(t *testing.T) {
		testShuffleRounding(t, 1, 10, 100)
	})
	t.Run("1% with blockSize 100", func(t *testing.T) {
		testShuffleRounding(t, 1, 100, 99)
	})
	t.Run("49% with blockSize 10", func(t *testing.T) {
		testShuffleRounding(t, 49, 10, 50)
	})
	t.Run("50% with blockSize 10", func(t *testing.T) {
		testShuffleRounding(t, 50, 10, 50)
	})
	t.Run("51% with blockSize 10", func(t *testing.T) {
		testShuffleRounding(t, 51, 10, 50)
	})
	t.Run("55% with blockSize 10", func(t *testing.T) {
		testShuffleRounding(t, 55, 10, 40)
	})
}

func setupLossFilterForResetTest(t *testing.T) (*LossFilter, *int) {
	t.Helper()
	mnic := newMockNIC(t)
	lossFilter, err := NewLossFilter(mnic, 10, WithShuffleLossHandler(100))
	if !assert.NoError(t, err, "should succeed") {
		return nil, nil
	}

	received := new(int)
	mnic.mockOnInboundChunk = func(Chunk) {
		(*received)++
	}

	return lossFilter, received
}

func TestLossFilterResetImmediately(t *testing.T) { //nolint:cyclop
	t.Run("resetImmediately true - applies immediately mid-block", func(t *testing.T) {
		lossFilter, received := setupLossFilterForResetTest(t)
		if lossFilter == nil {
			return
		}

		for i := 0; i < 30; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedBeforeReset := *received

		err := lossFilter.SetLossRate(90, true)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		for i := 0; i < 100; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedAfterReset := *received

		assert.GreaterOrEqual(t, receivedAfterReset, receivedBeforeReset+5)
		assert.LessOrEqual(t, receivedAfterReset, receivedBeforeReset+15)
	})

	t.Run("resetImmediately false - applies after current block", func(t *testing.T) {
		lossFilter, received := setupLossFilterForResetTest(t)
		if lossFilter == nil {
			return
		}

		for i := 0; i < 30; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedAt30 := *received

		err := lossFilter.SetLossRate(90, false)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		for i := 0; i < 70; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedAt100 := *received

		assert.GreaterOrEqual(t, receivedAt100, receivedAt30+60)
		assert.LessOrEqual(t, receivedAt100, receivedAt30+70)

		for i := 0; i < 100; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedAt200 := *received

		assert.GreaterOrEqual(t, receivedAt200, receivedAt100+5)
		assert.LessOrEqual(t, receivedAt200, receivedAt100+15)
	})

	t.Run("resetImmediately true multiple times", func(t *testing.T) {
		lossFilter, received := setupLossFilterForResetTest(t)
		if lossFilter == nil {
			return
		}

		for i := 0; i < 20; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		err := lossFilter.SetLossRate(50, true)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		for i := 0; i < 100; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedAt120 := *received

		err = lossFilter.SetLossRate(0, true)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		for i := 0; i < 100; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		assert.Equal(t, receivedAt120+100, *received)
	})

	t.Run("resetImmediately false then true", func(t *testing.T) {
		lossFilter, received := setupLossFilterForResetTest(t)
		if lossFilter == nil {
			return
		}

		for i := 0; i < 50; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		err := lossFilter.SetLossRate(90, false)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		for i := 0; i < 30; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}
		receivedAt80 := *received

		err = lossFilter.SetLossRate(0, true)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		for i := 0; i < 100; i++ {
			lossFilter.onInboundChunk(&chunkUDP{})
		}

		assert.Equal(t, receivedAt80+100, *received)
	})
}

func TestLossFilterWithSeed_DeterministicShuffle(t *testing.T) {
	t.Run("same seed produces same shuffle pattern", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)

		seed := int64(42)
		chance := 50
		blockSize := 20

		filter1, err := NewLossFilter(mnic1, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		dropped1 := make([]bool, blockSize*3)
		dropped2 := make([]bool, blockSize*3)

		for i := 0; i < blockSize*3; i++ {
			dropped1[i] = filter1.LossFilterHandler.shouldDrop()
			dropped2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, dropped1, dropped2, "same seed should produce identical shuffle patterns")
	})

	t.Run("different seeds produce different shuffle patterns", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)

		chance := 30
		blockSize := 20

		filter1, err := NewLossFilter(mnic1, chance, WithShuffleLossHandler(blockSize), WithLossSeed(1))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithShuffleLossHandler(blockSize), WithLossSeed(999))
		assert.NoError(t, err)

		dropped1 := make([]bool, blockSize*3)
		dropped2 := make([]bool, blockSize*3)

		for i := 0; i < blockSize*3; i++ {
			dropped1[i] = filter1.LossFilterHandler.shouldDrop()
			dropped2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.NotEqual(t, dropped1, dropped2, "different seeds should produce different shuffle patterns")
	})

	t.Run("shuffle order is deterministic with seed", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		seed := int64(12345)
		chance := 40
		blockSize := 10

		filter1, err := NewLossFilter(mnic1, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		pattern1 := make([]bool, blockSize)
		pattern2 := make([]bool, blockSize)

		for i := 0; i < blockSize; i++ {
			pattern1[i] = filter1.LossFilterHandler.shouldDrop()
			pattern2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, pattern1, pattern2, "shuffle order should be deterministic with same seed")
	})

	t.Run("shuffle maintains correct loss rate with seed", func(t *testing.T) {
		mnic := newMockNIC(t)
		seed := int64(777)
		chance := 25
		blockSize := 100

		filter, err := NewLossFilter(mnic, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		packets := blockSize * 5
		for i := 0; i < packets; i++ {
			filter.onInboundChunk(&chunkUDP{})
		}

		expectedReceived := (100 - chance) * 5
		assert.Equal(t, expectedReceived, received, "seed should not affect loss rate accuracy")
	})
}

func TestLossFilterWithSeed_DeterministicRandom(t *testing.T) {
	t.Run("same seed produces same random drop pattern", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)

		seed := int64(555)
		chance := 50

		filter1, err := NewLossFilter(mnic1, chance, WithLossSeed(seed))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithLossSeed(seed))
		assert.NoError(t, err)

		numPackets := 100
		dropped1 := make([]bool, numPackets)
		dropped2 := make([]bool, numPackets)

		for i := 0; i < numPackets; i++ {
			dropped1[i] = filter1.LossFilterHandler.shouldDrop()
			dropped2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, dropped1, dropped2, "same seed should produce identical random drop patterns")
	})

	t.Run("different seeds produce different random patterns", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)

		chance := 50

		filter1, err := NewLossFilter(mnic1, chance, WithLossSeed(100))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithLossSeed(200))
		assert.NoError(t, err)

		numPackets := 100
		dropped1 := make([]bool, numPackets)
		dropped2 := make([]bool, numPackets)

		for i := 0; i < numPackets; i++ {
			dropped1[i] = filter1.LossFilterHandler.shouldDrop()
			dropped2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.NotEqual(t, dropped1, dropped2, "different seeds should produce different random patterns")
	})

	t.Run("random loss maintains approximately correct rate with seed", func(t *testing.T) {
		mnic := newMockNIC(t)
		seed := int64(888)
		chance := 30

		filter, err := NewLossFilter(mnic, chance, WithLossSeed(seed))
		assert.NoError(t, err)

		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		packets := 10000
		for i := 0; i < packets; i++ {
			filter.onInboundChunk(&chunkUDP{})
		}

		expectedReceived := (100 - chance) * packets / 100
		tolerance := packets / 20 // 5% tolerance
		assert.InDelta(t, expectedReceived, received, float64(tolerance),
			"seed should maintain approximately correct loss rate")
	})
}

func TestLossFilterWithSeed_Reproducibility(t *testing.T) {
	t.Run("multiple runs with same seed are reproducible", func(t *testing.T) {
		mnic := newMockNIC(t)
		seed := int64(9999)
		chance := 35
		blockSize := 50

		filter1, err := NewLossFilter(mnic, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		pattern1 := make([]bool, blockSize*2)
		for i := 0; i < blockSize*2; i++ {
			pattern1[i] = filter1.LossFilterHandler.shouldDrop()
		}

		filter2, err := NewLossFilter(mnic, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		pattern2 := make([]bool, blockSize*2)
		for i := 0; i < blockSize*2; i++ {
			pattern2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, pattern1, pattern2, "multiple runs with same seed should be reproducible")
	})

	t.Run("seed works with custom handler", func(t *testing.T) {
		// Custom handlers built without an explicit seed use time-based seeding
		// and won't be deterministic. This test verifies that WithLossSeed forces
		// determinism.
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		seed := int64(1234)
		chance := 20
		blockSize := 50

		filter1, err := NewLossFilter(mnic1, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		pattern1 := make([]bool, 100)
		pattern2 := make([]bool, 100)

		for i := 0; i < 100; i++ {
			pattern1[i] = filter1.LossFilterHandler.shouldDrop()
			pattern2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, pattern1, pattern2, "filters with same seed should produce identical patterns")
	})
}

func TestLossFilterSeed_NoSeedRandomization(t *testing.T) {
	t.Run("no seed produces different results on different runs", func(t *testing.T) {
		mnic := newMockNIC(t)
		chance := 40

		filter, err := NewLossFilter(mnic, chance, WithShuffleLossHandler(100))
		assert.NoError(t, err)

		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		for i := 0; i < 100; i++ {
			filter.onInboundChunk(&chunkUDP{})
		}

		assert.Equal(t, 60, received, "shuffle should maintain correct loss rate even without explicit seed")
	})
}

func TestLossFilterOptionValidation(t *testing.T) {
	t.Run("invalid chance returns error", func(t *testing.T) {
		mnic := newMockNIC(t)
		_, err := NewLossFilter(mnic, -1)
		assert.ErrorIs(t, err, ErrInvalidChance)

		_, err = NewLossFilter(mnic, 101)
		assert.ErrorIs(t, err, ErrInvalidChance)
	})

	t.Run("invalid shuffleBlockSize returns error", func(t *testing.T) {
		mnic := newMockNIC(t)
		_, err := NewLossFilter(mnic, 50, WithShuffleLossHandler(0))
		assert.ErrorIs(t, err, ErrInvalidShuffleBlockSize)

		_, err = NewLossFilter(mnic, 50, WithShuffleLossHandler(-1))
		assert.ErrorIs(t, err, ErrInvalidShuffleBlockSize)
	})

	t.Run("WithLossHandler preserves handler settings", func(t *testing.T) {
		mnic := newMockNIC(t)
		customHandler := newShuffleLossHandle(10, 100, newRNG(nil))

		filter, err := NewLossFilter(mnic, 50, WithLossHandler(customHandler))
		assert.NoError(t, err)

		dropped := 0
		for i := 0; i < 100; i++ {
			if filter.LossFilterHandler.shouldDrop() {
				dropped++
			}
		}

		assert.Equal(t, 10, dropped)
	})
}

func TestLossFilterDeterminismProperty(t *testing.T) {
	t.Run("same seed produces identical drop sequences - random mode", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		seed := int64(42)
		chance := 50
		sequenceLength := 100

		filter1, err := NewLossFilter(mnic1, chance, WithLossSeed(seed))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithLossSeed(seed))
		assert.NoError(t, err)

		sequence1 := make([]bool, sequenceLength)
		sequence2 := make([]bool, sequenceLength)

		for i := 0; i < sequenceLength; i++ {
			sequence1[i] = filter1.LossFilterHandler.shouldDrop()
			sequence2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, sequence1, sequence2, "same seed should produce identical drop sequences")
	})

	t.Run("same seed produces identical drop sequences - shuffle mode", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		seed := int64(123)
		chance := 33
		blockSize := 30
		sequenceLength := 150 // 5 blocks

		filter1, err := NewLossFilter(mnic1, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithShuffleLossHandler(blockSize), WithLossSeed(seed))
		assert.NoError(t, err)

		sequence1 := make([]bool, sequenceLength)
		sequence2 := make([]bool, sequenceLength)

		for i := 0; i < sequenceLength; i++ {
			sequence1[i] = filter1.LossFilterHandler.shouldDrop()
			sequence2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, sequence1, sequence2, "same seed should produce identical shuffle sequences")
	})

	t.Run("different seeds produce different sequences - random mode", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		chance := 50
		sequenceLength := 100

		filter1, err := NewLossFilter(mnic1, chance, WithLossSeed(100))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithLossSeed(200))
		assert.NoError(t, err)

		sequence1 := make([]bool, sequenceLength)
		sequence2 := make([]bool, sequenceLength)

		for i := 0; i < sequenceLength; i++ {
			sequence1[i] = filter1.LossFilterHandler.shouldDrop()
			sequence2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.NotEqual(t, sequence1, sequence2, "different seeds should produce different sequences")
	})

	t.Run("different seeds produce different sequences - shuffle mode", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		chance := 40
		blockSize := 25
		sequenceLength := 100

		filter1, err := NewLossFilter(mnic1, chance, WithShuffleLossHandler(blockSize), WithLossSeed(1))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithShuffleLossHandler(blockSize), WithLossSeed(999))
		assert.NoError(t, err)

		sequence1 := make([]bool, sequenceLength)
		sequence2 := make([]bool, sequenceLength)

		for i := 0; i < sequenceLength; i++ {
			sequence1[i] = filter1.LossFilterHandler.shouldDrop()
			sequence2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.NotEqual(t, sequence1, sequence2, "different seeds should produce different shuffle sequences")
	})

	t.Run("zero seed is valid and deterministic", func(t *testing.T) {
		mnic1 := newMockNIC(t)
		mnic2 := newMockNIC(t)
		chance := 30
		sequenceLength := 50

		filter1, err := NewLossFilter(mnic1, chance, WithLossSeed(0))
		assert.NoError(t, err)

		filter2, err := NewLossFilter(mnic2, chance, WithLossSeed(0))
		assert.NoError(t, err)

		sequence1 := make([]bool, sequenceLength)
		sequence2 := make([]bool, sequenceLength)

		for i := 0; i < sequenceLength; i++ {
			sequence1[i] = filter1.LossFilterHandler.shouldDrop()
			sequence2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, sequence1, sequence2, "zero seed should produce deterministic sequences")
	})
}

func TestLossFilterBoundaryChances(t *testing.T) {
	t.Run("0% chance - random mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 0)
		assert.NoError(t, err)

		for i := 0; i < 1000; i++ {
			assert.False(t, filter.LossFilterHandler.shouldDrop(), "0% chance should never drop")
		}
	})

	t.Run("100% chance - random mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 100)
		assert.NoError(t, err)

		for i := 0; i < 1000; i++ {
			assert.True(t, filter.LossFilterHandler.shouldDrop(), "100% chance should always drop")
		}
	})

	t.Run("0% chance - shuffle mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 0, WithShuffleLossHandler(100))
		assert.NoError(t, err)

		for i := 0; i < 1000; i++ {
			assert.False(t, filter.LossFilterHandler.shouldDrop(), "0% chance should never drop in shuffle mode")
		}
	})

	t.Run("100% chance - shuffle mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 100, WithShuffleLossHandler(100))
		assert.NoError(t, err)

		for i := 0; i < 1000; i++ {
			assert.True(t, filter.LossFilterHandler.shouldDrop(), "100% chance should always drop in shuffle mode")
		}
	})

	t.Run("boundary with seed - random mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 0, WithLossSeed(42))
		assert.NoError(t, err)

		for i := 0; i < 100; i++ {
			assert.False(t, filter.LossFilterHandler.shouldDrop())
		}

		err = filter.SetLossRate(100, true)
		assert.NoError(t, err)

		for i := 0; i < 100; i++ {
			assert.True(t, filter.LossFilterHandler.shouldDrop())
		}
	})

	t.Run("boundary with seed - shuffle mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 0, WithShuffleLossHandler(50), WithLossSeed(42))
		assert.NoError(t, err)

		for i := 0; i < 100; i++ {
			assert.False(t, filter.LossFilterHandler.shouldDrop())
		}

		err = filter.SetLossRate(100, true)
		assert.NoError(t, err)

		for i := 0; i < 100; i++ {
			assert.True(t, filter.LossFilterHandler.shouldDrop())
		}
	})
}

func TestLossFilterRoundingComprehensive(t *testing.T) {
	testCases := []struct {
		name           string
		chance         int
		blockSize      int
		expectedDrops  int
		expectedRounds int
	}{
		{"33% of 10", 33, 10, 3, 3},
		{"67% of 10", 67, 10, 7, 7},
		{"33% of 3", 33, 3, 1, 1},
		{"67% of 3", 67, 3, 2, 2},
		{"25% of 4", 25, 4, 1, 1},
		{"75% of 4", 75, 4, 3, 3},
		{"10% of 7", 10, 7, 1, 1},
		{"90% of 7", 90, 7, 6, 6},
		{"5% of 20", 5, 20, 1, 1},
		{"15% of 20", 15, 20, 3, 3},
		{"85% of 20", 85, 20, 17, 17},
		{"95% of 20", 95, 20, 19, 19},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mnic := newMockNIC(t)
			filter, err := NewLossFilter(mnic, tc.chance, WithShuffleLossHandler(tc.blockSize))
			assert.NoError(t, err)

			// Test multiple rounds to ensure consistency
			for round := 0; round < tc.expectedRounds; round++ {
				dropped := 0
				for i := 0; i < tc.blockSize; i++ {
					if filter.LossFilterHandler.shouldDrop() {
						dropped++
					}
				}
				assert.Equal(t, tc.expectedDrops, dropped,
					"round %d: %d%% of %d should drop %d packets", round, tc.chance, tc.blockSize, tc.expectedDrops)
			}
		})
	}
}

func TestLossFilterConcurrencySmoke(t *testing.T) { //nolint:cyclop
	t.Run("concurrent shouldDrop calls - random mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 50, WithLossSeed(12345))
		assert.NoError(t, err)

		const numGoroutines = 50
		const callsPerGoroutine = 1000
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				for j := 0; j < callsPerGoroutine; j++ {
					_ = filter.LossFilterHandler.shouldDrop()
				}
				done <- true
			}()
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})

	t.Run("concurrent shouldDrop calls - shuffle mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 50, WithShuffleLossHandler(100), WithLossSeed(12345))
		assert.NoError(t, err)

		const numGoroutines = 50
		const callsPerGoroutine = 1000
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				for j := 0; j < callsPerGoroutine; j++ {
					_ = filter.LossFilterHandler.shouldDrop()
				}
				done <- true
			}()
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})

	t.Run("concurrent shouldDrop and setLossRate - random mode", func(t *testing.T) {
		mnic := newMockNIC(t)
		filter, err := NewLossFilter(mnic, 50, WithLossSeed(12345))
		assert.NoError(t, err)

		const numGoroutines = 20
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines/2; i++ {
			go func() {
				for j := 0; j < 1000; j++ {
					_ = filter.LossFilterHandler.shouldDrop()
				}
				done <- true
			}()
		}

		for i := 0; i < numGoroutines/2; i++ {
			go func() {
				for j := 0; j < 100; j++ {
					_ = filter.SetLossRate(j%101, false)
				}
				done <- true
			}()
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}

func TestLossFilterNilOptionSkip(t *testing.T) {
	t.Run("nil option is skipped", func(t *testing.T) {
		mnic := newMockNIC(t)
		seed := int64(42)

		filter, err := NewLossFilter(mnic, 50, nil, WithLossSeed(seed))
		assert.NoError(t, err)
		assert.NotNil(t, filter)

		sequence1 := make([]bool, 50)
		for i := 0; i < 50; i++ {
			sequence1[i] = filter.LossFilterHandler.shouldDrop()
		}

		filter2, err := NewLossFilter(mnic, 50, WithLossSeed(seed))
		assert.NoError(t, err)
		sequence2 := make([]bool, 50)
		for i := 0; i < 50; i++ {
			sequence2[i] = filter2.LossFilterHandler.shouldDrop()
		}

		assert.Equal(t, sequence1, sequence2, "nil option should be skipped and seed should still work")
	})

	t.Run("multiple nil options are skipped", func(t *testing.T) {
		mnic := newMockNIC(t)
		seed := int64(999)

		filter, err := NewLossFilter(mnic, 30, nil, nil, WithLossSeed(seed), nil)
		assert.NoError(t, err)
		assert.NotNil(t, filter)

		for i := 0; i < 100; i++ {
			_ = filter.LossFilterHandler.shouldDrop()
		}
	})

	t.Run("nil option with shuffle handler", func(t *testing.T) {
		mnic := newMockNIC(t)
		seed := int64(777)

		filter, err := NewLossFilter(mnic, 40, nil, WithShuffleLossHandler(50), WithLossSeed(seed))
		assert.NoError(t, err)
		assert.NotNil(t, filter)

		// Verify shuffle mode works
		dropped := 0
		for i := 0; i < 50; i++ {
			if filter.LossFilterHandler.shouldDrop() {
				dropped++
			}
		}
		assert.Equal(t, 20, dropped)
	})
}
