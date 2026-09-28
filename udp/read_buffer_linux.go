// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux

package udp

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// checkReadBufferSize verifies that the kernel actually applied the requested
// receive buffer size. Linux silently clamps SO_RCVBUF to net.core.rmem_max
// and reports the doubled (bookkeeping-inclusive) value on read back.
func checkReadBufferSize(conn *net.UDPConn, requested int) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var actual int
	var socketErr error
	if err = raw.Control(func(fd uintptr) {
		actual, socketErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF)
	}); err != nil {
		return err
	}
	if socketErr != nil {
		return socketErr
	}
	if applied := actual / 2; applied < requested {
		return fmt.Errorf("%w: requested %d bytes, applied %d bytes", ErrReadBufferCapped, requested, applied)
	}

	return nil
}
