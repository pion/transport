// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build darwin && !ios

package netchange

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openSocket() (notificationSource, error) {
	return openRoutingSocket(unix.Socket)
}

func openRoutingSocket(open func(int, int, int) (int, error)) (notificationSource, error) {
	// AF_UNSPEC receives interface and address notifications for both IP
	// families. Filter out routing-table activity before refreshing interfaces.
	syscall.ForkLock.RLock()
	fd, err := open(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err == nil {
		unix.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) ||
			errors.Is(err, unix.EAFNOSUPPORT) || errors.Is(err, unix.EPROTONOSUPPORT) || errors.Is(err, unix.EOPNOTSUPP) {
			return nil, nil //nolint:nilnil
		}

		return nil, fmt.Errorf("open routing socket: %w", err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)

		return nil, fmt.Errorf("set routing socket nonblocking: %w", err)
	}

	return &socket{file: os.NewFile(uintptr(fd), "route"), filter: interfaceNotification}, nil //nolint:gosec // fd is nonnegative.
}

func interfaceNotification(message []byte) bool {
	if len(message) < 4 || message[2] != unix.RTM_VERSION {
		return true // Refresh for an unknown message format.
	}
	switch message[3] {
	case unix.RTM_NEWADDR, unix.RTM_DELADDR, unix.RTM_IFINFO, unix.RTM_IFINFO2,
		unix.RTM_NEWMADDR, unix.RTM_DELMADDR, unix.RTM_NEWMADDR2:
		return true
	default:
		return false
	}
}
