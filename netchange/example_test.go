// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netchange_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pion/transport/v5/netchange"
)

func ExampleDetector_Check() {
	detector, err := netchange.NewDetector()
	if err != nil {
		fmt.Println(err)

		return
	}
	defer func() { _ = detector.Close() }()

	// the first Check reports initial interfaces,
	// later calls wait for changes.
	timeout := errors.New("network check timed out") //nolint:err113
	ctx, cancel := context.WithTimeoutCause(context.Background(), 30*time.Second, timeout)
	defer cancel()
	changes, err := detector.Check(ctx)
	if errors.Is(err, timeout) {
		fmt.Println("no change within 30 seconds")

		return
	}
	if err != nil {
		fmt.Println(err)

		return
	}
	for _, change := range changes {
		fmt.Println(change.Interface, change.Type)
	}
}

func ExampleWithInterfaceFilter() {
	detector, err := netchange.NewDetector(
		netchange.WithInterfaceFilter(
			func(name string) bool { return strings.HasPrefix(name, "eth") },
		),
	)
	if err != nil {
		fmt.Println(err)

		return
	}
	defer func() { _ = detector.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fmt.Println(detector.Check(ctx))
}
