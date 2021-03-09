package vnet_test

import (
	"net"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
)

// Proxy many vnet endpoint to one real server endpoint.
// For example:
//		vnet(10.0.0.11:5787) => proxy => 192.168.1.10:8000
//		vnet(10.0.0.11:5788) => proxy => 192.168.1.10:8000
//		vnet(10.0.0.11:5789) => proxy => 192.168.1.10:8000
func ExampleUDPProxyManyToOne() { // nolint:govet
	var clientNetwork *vnet.Net

	var serverAddr *net.UDPAddr
	if addr, err := net.ResolveUDPAddr("udp4", "192.168.1.10:8000"); err != nil {
		// handle error
	} else {
		serverAddr = addr
	}

	// Setup the network and proxy.
	if true {
		// Create vnet WAN with one endpoint, please read from
		// https://github.com/pion/transport/tree/master/vnet#example-wan-with-one-endpoint-vnet
		router, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR:          "0.0.0.0/0",
			LoggerFactory: logging.NewDefaultLoggerFactory(),
		})
		if err != nil {
			// handle error
		}

		// Create a network and add to router, for example, for client.
		clientNetwork = vnet.NewNet(&vnet.NetConfig{
			StaticIP: "10.0.0.11",
		})
		if err = router.AddNet(clientNetwork); err != nil {
			// handle error
		}

		// Start the router.
		if err = router.Start(); err != nil {
			// handle error
		}
		defer router.Stop() // nolint:errcheck

		// Create a proxy, bind to the router.
		proxy, err := vnet.NewProxy(router)
		if err != nil {
			// handle error
		}
		defer proxy.Close() // nolint:errcheck

		// Start to proxy some addresses, clientNetwork is a hit for proxy,
		// that the client in vnet is from this network.
		if err := proxy.Proxy(clientNetwork, serverAddr); err != nil {
			// handle error
		}
	}

	// Now, all packets from client, will be proxy to real server, vice versa.
	client0, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5787")
	if err != nil {
		// handle error
	}
	_, _ = client0.WriteTo([]byte("Hello"), serverAddr)

	client1, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5788")
	if err != nil {
		// handle error
	}
	_, _ = client1.WriteTo([]byte("Hello"), serverAddr)

	client2, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5789")
	if err != nil {
		// handle error
	}
	_, _ = client2.WriteTo([]byte("Hello"), serverAddr)
}

// Proxy many vnet endpoint to one real server endpoint.
// For example:
//		vnet(10.0.0.11:5787) => proxy => 192.168.1.10:8000
//		vnet(10.0.0.11:5788) => proxy => 192.168.1.10:8000
func ExampleUDPProxyMultileTimes() { // nolint:govet
	var clientNetwork *vnet.Net

	var serverAddr *net.UDPAddr
	if addr, err := net.ResolveUDPAddr("udp4", "192.168.1.10:8000"); err != nil {
		// handle error
	} else {
		serverAddr = addr
	}

	// Setup the network and proxy.
	var proxy *vnet.UDPProxy
	if true {
		// Create vnet WAN with one endpoint, please read from
		// https://github.com/pion/transport/tree/master/vnet#example-wan-with-one-endpoint-vnet
		router, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR:          "0.0.0.0/0",
			LoggerFactory: logging.NewDefaultLoggerFactory(),
		})
		if err != nil {
			// handle error
		}

		// Create a network and add to router, for example, for client.
		clientNetwork = vnet.NewNet(&vnet.NetConfig{
			StaticIP: "10.0.0.11",
		})
		if err = router.AddNet(clientNetwork); err != nil {
			// handle error
		}

		// Start the router.
		if err = router.Start(); err != nil {
			// handle error
		}
		defer router.Stop() // nolint:errcheck

		// Create a proxy, bind to the router.
		proxy, err = vnet.NewProxy(router)
		if err != nil {
			// handle error
		}
		defer proxy.Close() // nolint:errcheck
	}

	if true {
		// Start to proxy some addresses, clientNetwork is a hit for proxy,
		// that the client in vnet is from this network.
		if err := proxy.Proxy(clientNetwork, serverAddr); err != nil {
			// handle error
		}

		// Now, all packets from client, will be proxy to real server, vice versa.
		client0, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5787")
		if err != nil {
			// handle error
		}
		_, _ = client0.WriteTo([]byte("Hello"), serverAddr)
	}

	if true {
		// It's ok to proxy multiple times, for example, the publisher and player
		// might need to proxy when got answer.
		if err := proxy.Proxy(clientNetwork, serverAddr); err != nil {
			// handle error
		}

		client1, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5788")
		if err != nil {
			// handle error
		}
		_, _ = client1.WriteTo([]byte("Hello"), serverAddr)
	}
}

// Proxy one vnet endpoint to one real server endpoint.
// For example:
//		vnet(10.0.0.11:5787) => proxy0 => 192.168.1.10:8000
//		vnet(10.0.0.11:5788) => proxy1 => 192.168.1.10:8001
//		vnet(10.0.0.11:5789) => proxy2 => 192.168.1.10:8002
func ExampleUDPProxyOneToOne() { // nolint:govet
	var clientNetwork *vnet.Net

	var serverAddr0 *net.UDPAddr
	if addr, err := net.ResolveUDPAddr("udp4", "192.168.1.10:8000"); err != nil {
		// handle error
	} else {
		serverAddr0 = addr
	}

	var serverAddr1 *net.UDPAddr
	if addr, err := net.ResolveUDPAddr("udp4", "192.168.1.10:8001"); err != nil {
		// handle error
	} else {
		serverAddr1 = addr
	}

	var serverAddr2 *net.UDPAddr
	if addr, err := net.ResolveUDPAddr("udp4", "192.168.1.10:8002"); err != nil {
		// handle error
	} else {
		serverAddr2 = addr
	}

	// Setup the network and proxy.
	if true {
		// Create vnet WAN with one endpoint, please read from
		// https://github.com/pion/transport/tree/master/vnet#example-wan-with-one-endpoint-vnet
		router, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR:          "0.0.0.0/0",
			LoggerFactory: logging.NewDefaultLoggerFactory(),
		})
		if err != nil {
			// handle error
		}

		// Create a network and add to router, for example, for client.
		clientNetwork = vnet.NewNet(&vnet.NetConfig{
			StaticIP: "10.0.0.11",
		})
		if err = router.AddNet(clientNetwork); err != nil {
			// handle error
		}

		// Start the router.
		if err = router.Start(); err != nil {
			// handle error
		}
		defer router.Stop() // nolint:errcheck

		// Create a proxy, bind to the router.
		proxy, err := vnet.NewProxy(router)
		if err != nil {
			// handle error
		}
		defer proxy.Close() // nolint:errcheck

		// Start to proxy some addresses, clientNetwork is a hit for proxy,
		// that the client in vnet is from this network.
		if err := proxy.Proxy(clientNetwork, serverAddr0); err != nil {
			// handle error
		}
		if err := proxy.Proxy(clientNetwork, serverAddr1); err != nil {
			// handle error
		}
		if err := proxy.Proxy(clientNetwork, serverAddr2); err != nil {
			// handle error
		}
	}

	// Now, all packets from client, will be proxy to real server, vice versa.
	client0, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5787")
	if err != nil {
		// handle error
	}
	_, _ = client0.WriteTo([]byte("Hello"), serverAddr0)

	client1, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5788")
	if err != nil {
		// handle error
	}
	_, _ = client1.WriteTo([]byte("Hello"), serverAddr1)

	client2, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5789")
	if err != nil {
		// handle error
	}
	_, _ = client2.WriteTo([]byte("Hello"), serverAddr2)
}
