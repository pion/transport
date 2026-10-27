// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements an example for simple NAT traversal
package main

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/transport/v5/vnet"
)

func main() {
	// the fake internet, everything hangs off this
	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		panic(err)
	}

	// same nat settings for both home networks, so just reuse this
	natCfg := func() *vnet.NATType {
		return &vnet.NATType{
			MappingBehavior:   vnet.EndpointIndependent,
			FilteringBehavior: vnet.EndpointIndependent,
			MappingLifeTime:   time.Second * 30,
		}
	}

	// home network 1
	childrouter, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "192.168.1.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
		NATType:       natCfg(),
	})
	if err != nil {
		panic(err)
	}

	// home network 2
	childrouter2, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "192.168.2.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
		NATType:       natCfg(),
	})
	if err != nil {
		panic(err)
	}

	wan.AddRouter(childrouter)  // nolint:errcheck
	wan.AddRouter(childrouter2) // nolint:errcheck

	// one device behind each nat, ips get picked for us
	device1, err := vnet.NewNet(&vnet.NetConfig{})
	if err != nil {
		panic(err)
	}
	device2, err := vnet.NewNet(&vnet.NetConfig{})
	if err != nil {
		panic(err)
	}

	childrouter.AddNet(device1)  // nolint:errcheck
	childrouter2.AddNet(device2) // nolint:errcheck

	// the stun server lives on the "internet" with a fixed ip
	stunNet, err := vnet.NewNet(&vnet.NetConfig{
		StaticIPs: []string{"1.2.3.4"},
	})
	if err != nil {
		panic(err)
	}
	wan.AddNet(stunNet) // nolint:errcheck

	if err = wan.Start(); err != nil {
		panic(err)
	}
	defer wan.Stop() // nolint:errcheck

	// tiny stun server, only answers binding requests
	stunConn, err := stunNet.ListenPacket("udp4", "1.2.3.4:3478")
	if err != nil {
		panic(err)
	}
	defer stunConn.Close() // nolint:errcheck

	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := stunConn.ReadFrom(buf)
			if err != nil {
				return
			}

			// need Decode() here, without it msg.Type is never filled in
			// and every packet gets skipped (this was the first bug)
			msg := &stun.Message{Raw: append([]byte{}, buf[:n]...)}
			if err := msg.Decode(); err != nil {
				continue
			}
			if msg.Type != stun.BindingRequest {
				continue
			}

			udpAddr, ok := addr.(*net.UDPAddr)
			if !ok {
				continue
			}

			// reply has to reuse the request's transaction id
			// or the client will ignore it
			resp, err := stun.Build(
				stun.NewTransactionIDSetter(msg.TransactionID),
				stun.BindingSuccess,
				&stun.XORMappedAddress{IP: udpAddr.IP, Port: udpAddr.Port},
				stun.Fingerprint,
			)
			if err != nil {
				continue
			}
			_, _ = stunConn.WriteTo(resp.Raw, addr)
		}
	}()

	fmt.Println("Started stun server")

	// open the sockets first and keep them open. the socket that talks
	// to stun must be the same one we punch with, otherwise the nat
	// gives us a different public port and nothing arrives
	conn1, err := device1.ListenPacket("udp4", ":0")
	if err != nil {
		panic(err)
	}
	defer conn1.Close() // nolint:errcheck

	conn2, err := device2.ListenPacket("udp4", ":0")
	if err != nil {
		panic(err)
	}
	defer conn2.Close() // nolint:errcheck

	// ask stun what our public address looks like
	addr1 := getSrflx(conn1, "1.2.3.4:3478")
	addr2 := getSrflx(conn2, "1.2.3.4:3478")
	fmt.Printf("device 1 public: %s\n", addr1)
	fmt.Printf("device 2 public: %s\n", addr2)

	// punch towards each other's public address.
	// sending a few times since stricter nats drop the first packet
	for i := 0; i < 3; i++ {
		_, _ = conn1.WriteTo([]byte("hole-punch-from-A"), mustResolve(addr2))
		_, _ = conn2.WriteTo([]byte("hole-punch-from-B"), mustResolve(addr1))
		time.Sleep(100 * time.Millisecond)
	}

	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))

	// if we get the message the hole punch worked
	bufA := make([]byte, 1500)
	if nA, fromA, errA := conn1.ReadFrom(bufA); errA == nil {
		fmt.Printf("1 received: %s from %s\n", string(bufA[:nA]), fromA)
	} else {
		fmt.Printf("1 not received: %v\n", errA)
	}

	bufB := make([]byte, 1500)
	if nB, fromB, errB := conn2.ReadFrom(bufB); errB == nil {
		fmt.Printf("2 received: %s from %s\n", string(bufB[:nB]), fromB)
	} else {
		fmt.Printf("2 not received: %v\n", errB)
	}
}

// getSrflx sends a stun binding request on a socket we already have
// and returns the public address the server saw. doesn't close the socket.
func getSrflx(conn net.PacketConn, stunAddr string) string {
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.WriteTo(msg.Raw, mustResolve(stunAddr)); err != nil {
		panic(err)
	}

	buf := make([]byte, 1500)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		panic(fmt.Sprintf("STUN no response: %v", err))
	}

	// reset the deadline so it doesn't mess with reads later
	_ = conn.SetReadDeadline(time.Time{})

	resp := &stun.Message{Raw: append([]byte{}, buf[:n]...)}
	if err = resp.Decode(); err != nil {
		panic(err)
	}

	// make sure this is actually the answer to our request
	if resp.TransactionID != msg.TransactionID {
		panic("STUN transaction ID mismatch")
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(resp); err != nil {
		panic(err)
	}
	return xorAddr.String()
}

// small helper so we don't repeat the error check everywhere
func mustResolve(addr string) *net.UDPAddr {
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	return a
}