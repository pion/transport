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
