// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !linux

package netchange

func openSocket() (notificationSource, error) {
	return nil, nil //nolint:nilnil
}
