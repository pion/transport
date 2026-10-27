// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !linux

package udp

import "net"

func checkReadBufferSize(*net.UDPConn, int) error {
	return nil
}
