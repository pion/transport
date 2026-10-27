// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux

package udp

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// One byte over rmem_max, or a huge fallback when the sysctl is not readable.
func cappedReadBufferSize(t *testing.T) int {
	t.Helper()

	data, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		return 1 << 30
	}
	limit, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)

	return limit + 1
}

func TestCheckReadBufferSizeApplied(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	const size = 64 * 1024
	require.NoError(t, conn.SetReadBuffer(size))
	require.NoError(t, checkReadBufferSize(conn, size))
}

func TestListenConfigRejectsCappedReadBuffer(t *testing.T) {
	listener, err := (&ListenConfig{ReadBufferSize: cappedReadBufferSize(t)}).
		Listen("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.Nil(t, listener)
	require.ErrorIs(t, err, ErrReadBufferCapped)
}
