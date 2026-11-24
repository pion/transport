// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package udp

import (
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

func TestBatchConnUDPLifecycle(t *testing.T) {
	for _, address := range []struct{ network, host string }{
		{"udp4", "127.0.0.1:0"}, {"udp6", "[::1]:0"},
	} {
		t.Run(address.network, func(t *testing.T) {
			config := net.ListenConfig{}
			receiver, err := config.ListenPacket(t.Context(), address.network, address.host)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, receiver.Close()) })
			socket, err := config.ListenPacket(t.Context(), address.network, address.host)
			require.NoError(t, err)
			conn := NewBatchConn(socket, 4, time.Millisecond)
			t.Cleanup(func() {
				if closeErr := conn.Close(); closeErr != nil {
					require.ErrorIs(t, closeErr, net.ErrClosed)
				}
			})
			linuxBatch := runtime.GOOS == "linux"
			require.Equal(t, linuxBatch, conn.batchConn != nil)
			if conn.batchConn == nil {
				// Exercise the queue using x/net's single-message fallback on other platforms.
				conn.batchConn = ipv4.NewPacketConn(socket)
				conn.batchPingPong = newPingPong(4, time.Millisecond, conn.batchConn)
			}
			t.Logf("Linux batch syscalls: %t", linuxBatch)
			for sequence := range byte(13) {
				payload := []byte{sequence, 0xAB}
				n, writeErr := conn.WriteTo(payload, receiver.LocalAddr())
				require.NoError(t, writeErr)
				require.Equal(t, len(payload), n)
				payload[0] = 0xFF
			}
			require.NoError(t, receiver.SetReadDeadline(time.Now().Add(time.Second)))
			for sequence := range byte(13) {
				buffer := make([]byte, 64)
				n, _, readErr := receiver.ReadFrom(buffer)
				require.NoError(t, readErr)
				require.Equal(t, []byte{sequence, 0xAB}, buffer[:n])
			}
			require.NoError(t, conn.Close())
			n, writeErr := conn.WriteTo([]byte{1}, receiver.LocalAddr())
			require.Zero(t, n)
			require.ErrorIs(t, writeErr, net.ErrClosed)
		})
	}
}
