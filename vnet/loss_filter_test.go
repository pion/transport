// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"testing"

	"github.com/pion/transport/v3"
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

	lossHandler, err := NewRandomShuffleLossHandler(10, 100)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	lossFilter, err := NewLossFilterWithOptions(mnic, 0, WithLossHandler(lossHandler))
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

	lossHandler, err := NewRandomShuffleLossHandler(10, 100)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	lossFilter, err := NewLossFilterWithOptions(mnic, 0, WithLossHandler(lossHandler))
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

	lossHandler, err := NewRandomShuffleLossHandler(10, 100)
	if !assert.NoError(t, err, "should succeed") {
		return
	}

	lossFilter, err := NewLossFilterWithOptions(mnic, 0, WithLossHandler(lossHandler))
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

		customHandler, err := NewRandomShuffleLossHandler(10, 100)
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		// Using options pattern
		lossFilter, err := NewLossFilterWithOptions(mnic, 50, WithLossHandler(customHandler))
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
		lossFilter, err := NewLossFilterWithOptions(mnic, 25, WithShuffleLossHandler(100))
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
	lossFilter, err := NewLossFilterWithOptions(mnic, chance, WithShuffleLossHandler(blockSize))
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
	lossFilter, err := NewLossFilterWithOptions(mnic, 10, WithShuffleLossHandler(100))
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
