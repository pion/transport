// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux

package netchange

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openSocket() (notificationSource, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("open netlink socket: %w", err)
	}
	address := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR,
	}
	if err = unix.Bind(fd, address); err != nil {
		_ = unix.Close(fd)

		return nil, fmt.Errorf("subscribe to netlink notifications: %w", err)
	}

	return &socket{file: os.NewFile(uintptr(fd), "netlink")}, nil //nolint:gosec // fd is nonnegative.
}
