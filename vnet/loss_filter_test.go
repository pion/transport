package vnet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockNIC struct {
	mockGetInterface   func(ifName string) (*Interface, error)
	mockOnInboundChunk func(c Chunk)
	mockGetStaticIPs   func() []net.IP
	mockSetRouter      func(r *Router) error
}

func (n *mockNIC) getInterface(ifName string) (*Interface, error) {
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
	return &mockNIC{
		mockGetInterface: func(string) (*Interface, error) {
			assert.Fail(t, "unexpceted call to mockGetInterface")
			return nil, nil
		},
		mockOnInboundChunk: func(Chunk) {
			assert.Fail(t, "unexpceted call to mockOnInboundChunk")
		},
		mockGetStaticIPs: func() []net.IP {
			assert.Fail(t, "unexpceted call to mockGetStaticIPs")
			return nil
		},
		mockSetRouter: func(*Router) error {
			assert.Fail(t, "unexpceted call to mockSetRouter")
			return nil
		},
	}
}

func TestLossFilter(t *testing.T) {
	t.Run("FullLoss", func(t *testing.T) {
		mnic := newMockNIC(t)

		f, err := NewLossFilter(mnic, 100)
		assert.NoError(t, err)

		f.onInboundChunk(&chunkUDP{})
	})

	t.Run("NoLoss", func(t *testing.T) {
		mnic := newMockNIC(t)

		f, err := NewLossFilter(mnic, 0)
		assert.NoError(t, err)

		packets := 100
		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		for i := 0; i < packets; i++ {
			f.onInboundChunk(&chunkUDP{})
		}

		assert.Equal(t, packets, received)
	})

	t.Run("SomeLoss", func(t *testing.T) {
		mnic := newMockNIC(t)

		f, err := NewLossFilter(mnic, 50)
		assert.NoError(t, err)

		packets := 1000
		received := 0
		mnic.mockOnInboundChunk = func(Chunk) {
			received++
		}

		for i := 0; i < packets; i++ {
			f.onInboundChunk(&chunkUDP{})
		}

		// One of the following could technically fail, but very unlikely
		assert.Less(t, 0, received)
		assert.Greater(t, packets, received)
	})
}
