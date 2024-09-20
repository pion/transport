// SPDX-FileCopyrightText: 2013 The Go Authors. All rights reserved.
// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2024 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package xor provides the XorBytes function.
package xor

import (
	"crypto/subtle"
)

// XorBytes calls [crypto/suble.XORBytes].
//
// Deprecated: please call [crypto/subtle.XORBytes] instead.
//
//revive:disable-next-line
func XorBytes(dst, a, b []byte) int {
	return subtle.XORBytes(dst, a, b)
}
