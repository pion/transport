// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkQueue(t *testing.T) {
	chunk := newChunkUDP(&net.UDPAddr{
		IP:   net.ParseIP("192.188.0.2"),
		Port: 1234,
	}, &net.UDPAddr{
		IP:   net.ParseIP(demoIP),
		Port: 5678,
	})
	chunk.userData = make([]byte, 1200)

	var ok bool
	var queue *chunkQueue
	var chunk2 Chunk

	queue = newChunkQueue(0, 0)

	chunk2 = queue.peek()
	assert.Nil(t, chunk2, "should return nil")

	ok = queue.push(chunk)
	assert.True(t, ok, "should succeed")

	chunk2, ok = queue.pop()
	assert.True(t, ok, "should succeed")
	assert.Equal(t, chunk, chunk2, "should be the same")

	chunk2, ok = queue.pop()
	assert.False(t, ok, "should fail")
	assert.Nil(t, chunk2, "should be nil")

	queue = newChunkQueue(1, 0)
	ok = queue.push(chunk)
	assert.True(t, ok, "should succeed")

	ok = queue.push(chunk)
	assert.False(t, ok, "should fail")

	chunk2 = queue.peek()
	assert.Equal(t, chunk, chunk2, "should be the same")

	queue = newChunkQueue(0, 1500)
	ok = queue.push(chunk)
	assert.True(t, ok, "should succeed")

	ok = queue.push(chunk)
	assert.False(t, ok, "should fail")

	chunk2 = queue.peek()
	assert.Equal(t, chunk, chunk2, "should be the same")
}
