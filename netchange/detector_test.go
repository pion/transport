// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netchange

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/pion/transport/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckInitialInterfaces(t *testing.T) {
	for _, name := range []string{"eth0", ""} {
		t.Run("initial="+name, func(t *testing.T) {
			detector := &Detector{enumerate: func() ([]interfaceState, error) {
				if name == "" {
					return nil, nil
				}

				return []interfaceState{{Index: 1, Name: name}}, nil
			}}
			require.NoError(t, detector.start())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			changes, err := detector.Check(ctx)
			require.NoError(t, err)
			if name == "" {
				assert.Empty(t, changes)
			} else {
				assert.Equal(t, []Change{{Interface: name, Type: Added}}, changes)
			}
			ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			changes, err = detector.Check(ctx)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Empty(t, changes, "initial results must only be delivered once")
		})
	}
}

func TestInterfaceChanges(t *testing.T) {
	detector := &Detector{}
	WithInterfaceFilter(func(name string) bool { return name != "ignored" })(detector)
	state := []interfaceState{{Index: 1, Name: "kept"}, {Index: 2, Name: "removed"}}
	detector.state.update(state)
	next := []interfaceState{
		{Index: 3, Name: "added"},
		{Index: 1, Name: "kept", Addrs: map[netip.Prefix]struct{}{netip.MustParsePrefix("192.0.2.1/24"): {}}},
		{Index: 4, Name: "ignored"},
	}
	assert.Equal(t, []Change{
		{Interface: "kept", Type: Changed},
		{Interface: "added", Type: Added},
		{Interface: "removed", Type: Removed},
	}, detector.state.update(next))
	assert.Empty(t, detector.state.update(next), "unchanged state must not produce events")
}

func TestCheckCancellationPreservesChange(t *testing.T) {
	state := []interfaceState{{Index: 1, Name: "before"}}
	detector := &Detector{enumerate: func() ([]interfaceState, error) { return state, nil }}
	require.NoError(t, detector.start())
	_, err := detector.Check(context.Background())
	require.NoError(t, err)
	cause := errors.New("parent stopped") //nolint:err113 // Unique cancellation cause for this test.
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	state[0].Name = "after" //nolint:goconst
	detector.enumerate = func() ([]interfaceState, error) {
		cancel(cause)

		return state, nil
	}
	changes, err := detector.Check(ctx)
	require.ErrorIs(t, err, cause)
	assert.Empty(t, changes)
	changes, err = detector.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Change{{Interface: "after", Type: Changed}}, changes)
}

func TestCheckUpdatesNet(t *testing.T) {
	detector, err := NewDetector(
		WithInterfaceFilter(func(string) bool { return false }),
		WithPlatformTimeout(5*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { _ = detector.Close() }()
	before, err := detector.Interfaces()
	require.NoError(t, err)
	if len(before) == 0 {
		t.Skip("no network interfaces available")
	}
	var network transport.Net = detector
	_, err = detector.Check(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	changes, err := detector.Check(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, changes, "event filters must not filter the network snapshot")
	interfaces, err := network.Interfaces()
	require.NoError(t, err)
	require.NotEmpty(t, interfaces)
	require.NotSame(t, before[0], interfaces[0], "refresh must publish a new snapshot")
	byIndex, err := network.InterfaceByIndex(interfaces[0].Index)
	require.NoError(t, err)
	require.Same(t, interfaces[0], byIndex)
	byName, err := network.InterfaceByName(interfaces[0].Name)
	require.NoError(t, err)
	require.Same(t, interfaces[0], byName)
	require.NoError(t, detector.Close())
	afterClose, err := network.Interfaces()
	require.NoError(t, err)
	require.Same(t, interfaces[0], afterClose[0], "last interfaces remain available after close")
}
