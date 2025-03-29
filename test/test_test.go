// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package test

import (
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStressIOPipe(t *testing.T) {
	r, w := io.Pipe()

	opt := Options{
		MsgSize:  2048,
		MsgCount: 100,
	}

	assert.NoError(t, Stress(w, r, opt))
}

func TestStressDuplexNetPipe(t *testing.T) {
	ca, cb := net.Pipe()

	opt := Options{
		MsgSize:  2048,
		MsgCount: 100,
	}

	assert.NoError(t, StressDuplex(ca, cb, opt))
}

func BenchmarkPipe(b *testing.B) {
	ca, cb := net.Pipe()

	b.ResetTimer()

	opt := Options{
		MsgSize:  2048,
		MsgCount: b.N,
	}

	assert.NoError(b, Stress(ca, cb, opt))
}
