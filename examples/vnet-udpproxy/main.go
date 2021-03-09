package main

import (
	"flag"
	"net"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
)

func main() {
	address := flag.String("address", "", "Destination address that three separate vnet clients will send too")
	flag.Parse()

	// Create vnet WAN with one endpoint
	// See the following docs for more information
	// https://github.com/pion/transport/tree/master/vnet#example-wan-with-one-endpoint-vnet
	router, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "0.0.0.0/0",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		panic(err)
	}

	// Create a network and add to router, for example, for client.
	clientNetwork := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "10.0.0.11",
	})
	if err = router.AddNet(clientNetwork); err != nil {
		panic(err)
	}

	if err = router.Start(); err != nil {
		panic(err)
	}
	defer router.Stop() // nolint:errcheck

	// Create a proxy, bind to the router.
	proxy, err := vnet.NewProxy(router)
	if err != nil {
		panic(err)
	}
	defer proxy.Close() // nolint:errcheck

	serverAddr, err := net.ResolveUDPAddr("udp4", *address)
	if err != nil {
		panic(err)
	}

	// Start to proxy some addresses, clientNetwork is a hit for proxy,
	// that the client in vnet is from this network.
	if err = proxy.Proxy(clientNetwork, serverAddr); err != nil {
		panic(err)
	}

	// Now, all packets from client, will be proxy to real server, vice versa.
	client0, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5787")
	if err != nil {
		panic(err)
	}
	_, _ = client0.WriteTo([]byte("Hello"), serverAddr)

	client1, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5788")
	if err != nil {
		panic(err)
	}
	_, _ = client1.WriteTo([]byte("Hello"), serverAddr)

	client2, err := clientNetwork.ListenPacket("udp4", "10.0.0.11:5789")
	if err != nil {
		panic(err)
	}
	_, _ = client2.WriteTo([]byte("Hello"), serverAddr)

	// Packets are delivered by a goroutine so WriteTo
	// return doesn't mean success. This may improve in
	// the future.
	time.Sleep(time.Second * 3)
}
