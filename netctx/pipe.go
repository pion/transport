// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netctx

import (
	"net"
)

// Pipe creates piped pair of ConnCtx.
func Pipe() (ConnCtx, ConnCtx) {
	ca, cb := net.Pipe()
	return NewConnCtx(ca), NewConnCtx(cb)
}
