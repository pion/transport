// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build unix || windows

package reuseport

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testDialFromListeningTCPPort(t *testing.T, network, host string) {
	t.Helper()

	lc := net.ListenConfig{
		Control: Control,
	}
	ctx := context.Background()

	// Allocate a listener that supplies the source address.
	ll, err := lc.Listen(ctx, network, host+":0")
	assert.NoError(t, err)

	// Allocate another listener that supplies the destination address.
	rl, err := lc.Listen(ctx, network, host+":0")
	assert.NoError(t, err)

	// Create a dialer do bind to local address.
	d := net.Dialer{
		LocalAddr: ll.Addr(),
		Control:   Control,
	}

	// Dial.
	c, err := d.Dial(network, rl.Addr().String())
	assert.NoError(t, err)

	assert.NoError(t, c.Close())
}

func testDialFromListeningUDPPort(t *testing.T, network, host string) {
	t.Helper()

	lc := net.ListenConfig{
		Control: Control,
	}
	ctx := context.Background()

	// Allocate a listener that supplies the source address.
	ll, err := lc.ListenPacket(ctx, network, host+":0")
	assert.NoError(t, err)

	// Allocate another listener that supplies the destination address.
	rl, err := lc.ListenPacket(ctx, network, host+":0")
	assert.NoError(t, err)

	// Create a dialer do bind to local address.
	d := net.Dialer{
		LocalAddr: ll.LocalAddr(),
		Control:   Control,
	}

	// Dial.
	c, err := d.Dial(network, rl.LocalAddr().String())
	assert.NoError(t, err)

	assert.NoError(t, c.Close())
}

func TestDialFromListeningPortTCP4(t *testing.T) {
	testDialFromListeningTCPPort(t, "tcp", "localhost")
}

func TestDialFromListeningPortTCP6(t *testing.T) {
	testDialFromListeningTCPPort(t, "tcp6", "[::1]")
}

func TestDialFromListeningPortUdp4(t *testing.T) {
	testDialFromListeningUDPPort(t, "udp", "localhost")
}

func TestDialFromListeningPortUdp6(t *testing.T) {
	testDialFromListeningUDPPort(t, "udp6", "[::1]")
}
