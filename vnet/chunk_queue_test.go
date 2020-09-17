package vnet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkQueue(t *testing.T) {
	c := newChunkUDP(&net.UDPAddr{
		IP:   net.ParseIP("192.188.0.2"),
		Port: 1234,
	}, &net.UDPAddr{
		IP:   net.ParseIP(demoIP),
		Port: 5678,
	})

	var ok bool
	var q *chunkQueue
	var d Chunk

	q = newChunkQueue(0)

	d = q.peek()
	assert.Nil(t, d, "should return nil")

	ok = q.push(c)
	assert.True(t, ok, "should succeed")

	d, ok = q.pop()
	assert.True(t, ok, "should succeed")
	assert.Equal(t, c, d, "should be the same")

	d, ok = q.pop()
	assert.False(t, ok, "should fail")
	assert.Nil(t, d, "should be nil")

	q = newChunkQueue(1)
	ok = q.push(c)
	assert.True(t, ok, "should succeed")

	ok = q.push(c)
	assert.False(t, ok, "should fail")

	d = q.peek()
	assert.Equal(t, c, d, "should be the same")
}
