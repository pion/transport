// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build darwin && !ios

package netchange

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRoutingSocketFallback(t *testing.T) {
	for _, socketErr := range []error{unix.EACCES, unix.EPERM, unix.EAFNOSUPPORT, unix.EPROTONOSUPPORT, unix.EOPNOTSUPP, unix.EMFILE} {
		t.Run(socketErr.Error(), func(t *testing.T) {
			source, err := openRoutingSocket(func(int, int, int) (int, error) { return -1, socketErr })
			require.Nil(t, source)
			if errors.Is(socketErr, unix.EMFILE) {
				require.ErrorIs(t, err, socketErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRoutingNotifications(t *testing.T) {
	for _, messageType := range []byte{
		unix.RTM_NEWADDR, unix.RTM_DELADDR, unix.RTM_IFINFO, unix.RTM_IFINFO2,
		unix.RTM_NEWMADDR, unix.RTM_DELMADDR, unix.RTM_NEWMADDR2,
	} {
		require.True(t, interfaceNotification([]byte{4, 0, unix.RTM_VERSION, messageType}))
	}
	for _, messageType := range []byte{unix.RTM_ADD, unix.RTM_DELETE, unix.RTM_CHANGE, unix.RTM_REDIRECT, unix.RTM_MISS, unix.RTM_GET} {
		require.False(t, interfaceNotification([]byte{4, 0, unix.RTM_VERSION, messageType}))
	}
	require.True(t, interfaceNotification(nil))
	require.True(t, interfaceNotification([]byte{4, 0, 0, unix.RTM_GET}))
	sender, receiver := newSocketPair(t)
	receiver.filter = interfaceNotification
	route := []byte{4, 0, unix.RTM_VERSION, unix.RTM_ADD}
	address := []byte{4, 0, unix.RTM_VERSION, unix.RTM_NEWADDR}
	require.NoError(t, unix.Send(sender, route, 0))
	notified, err := receiver.drain()
	require.NoError(t, err)
	require.False(t, notified)
	require.NoError(t, unix.Send(sender, address, 0))
	notified, err = receiver.drain()
	require.NoError(t, err)
	require.True(t, notified)

	require.NoError(t, unix.Send(sender, route, 0))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, receiver.wait(ctx), context.DeadlineExceeded)

	require.NoError(t, unix.Send(sender, route, 0))
	require.NoError(t, unix.Send(sender, address, 0))
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, receiver.wait(ctx))
}
