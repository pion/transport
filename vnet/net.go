package vnet

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
)

var macAddrCounter uint64 = 0xBEEFED910200

func isAnyIP(ip net.IP) bool {
	switch ip.String() {
	case "0.0.0.0":
		return true
	case "::":
		return true
	default:
	}
	return false
}

func newMACAddress() net.HardwareAddr {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, macAddrCounter)
	macAddrCounter++
	return b[2:]
}

type vNet struct {
	interfaces []*Interface
	staticIP   net.IP
	router     *Router
	udpConns   *udpConnMap
	mutex      sync.RWMutex
}

func (v *vNet) _getInterfaces() ([]*Interface, error) {
	if len(v.interfaces) == 0 {
		return nil, fmt.Errorf("no interface is available")
	}

	return v.interfaces, nil
}

func (v *vNet) getInterfaces() ([]*Interface, error) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return v._getInterfaces()
}

// caller must hold the mutex (read)
func (v *vNet) _getInterface(ifName string) (*Interface, error) {
	ifs, err := v._getInterfaces()
	if err != nil {
		return nil, err
	}
	for _, ifc := range ifs {
		if ifc.Name == ifName {
			return ifc, nil
		}
	}

	return nil, fmt.Errorf("interface %s not found", ifName)
}

func (v *vNet) getInterface(ifName string) (*Interface, error) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return v._getInterface(ifName)
}

// caller must hold the mutex
func (v *vNet) getAllIPAddrs(ipv6 bool) []net.IP {
	ips := []net.IP{}

	for _, ifc := range v.interfaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			if ipNet, ok := addr.(*net.IPNet); ok {
				ip = ipNet.IP
			} else if ipAddr, ok := addr.(*net.IPAddr); ok {
				ip = ipAddr.IP
			} else {
				continue
			}

			if !ipv6 {
				if ip.To4() != nil {
					ips = append(ips, ip)
				}
			}
		}
	}

	return ips
}

func (v *vNet) setRouter(r *Router) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.router = r
	return nil
}

func (v *vNet) onInboundChunk(c Chunk) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if c.Network() == "udp" {
		if conn, ok := v.udpConns.find(c.DestinationAddr()); ok {
			select {
			case conn.readCh <- c:
			default:
			}
		}
	}
}

// caller must hold the mutex
func (v *vNet) _listenUDP(network string, locAddr *net.UDPAddr) (UDPPacketConn, error) {
	// validate network
	if network != "udp" && network != "udp4" {
		return nil, fmt.Errorf("unexpected network: %s", network)
	}

	// validate address. do we have that address?
	if !v.hasIPAddr(locAddr.IP) {
		return nil, &net.OpError{
			Op:   "listen",
			Net:  network,
			Addr: locAddr,
			Err:  fmt.Errorf("bind: can't assign requested address"),
		}
	}

	if locAddr.Port == 0 {
		// choose randomly from the range between 5000 and 5999
		port, err := v.assignPort(locAddr.IP, 5000, 5999)
		if err != nil {
			return nil, &net.OpError{
				Op:   "listen",
				Net:  network,
				Addr: locAddr,
				Err:  err,
			}
		}
		locAddr.Port = port
	} else {
		if _, ok := v.udpConns.find(locAddr); ok {
			return nil, &net.OpError{
				Op:   "listen",
				Net:  network,
				Addr: locAddr,
				Err:  fmt.Errorf("bind: address already in use"),
			}
		}
	}

	conn, err := newUDPConn(locAddr, v)
	if err != nil {
		return nil, err
	}

	err = v.udpConns.insert(conn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (v *vNet) listenPacket(network string, address string) (UDPPacketConn, error) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	udpAddr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, err
	}

	return v._listenUDP(network, udpAddr)
}

func (v *vNet) listenUDP(network string, locAddr *net.UDPAddr) (UDPPacketConn, error) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	return v._listenUDP(network, locAddr)
}

func (v *vNet) dial(network string, address string) (net.Conn, error) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	host, sPort, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// Check if host is a domain name
	ip := net.ParseIP(host)
	if ip == nil {
		// host is a domain name. resolve IP address by the name
		if v.router == nil {
			return nil, fmt.Errorf("no router linked")
		}

		ip, err = v.router.resolver.lookUp(host)
		if err != nil {
			return nil, err
		}
	}

	port, err := strconv.Atoi(sPort)
	if err != nil {
		return nil, fmt.Errorf("invalid port number")
	}
	remAddr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}

	// Determine source address
	var srcIP net.IP
	if remAddr.IP.IsLoopback() {
		srcIP = net.ParseIP("127.0.0.1")
	} else {
		ifc, err2 := v._getInterface("eth0")
		if err2 != nil {
			return nil, err2
		}

		addrs, err2 := ifc.Addrs()
		if err2 != nil {
			return nil, err2
		}

		if len(addrs) == 0 {
			return nil, fmt.Errorf("no IP address available for eth0")
		}

		srcIP = addrs[0].(*net.IPNet).IP
	}

	// assign a port number
	port, err = v.assignPort(srcIP, 5000, 5999)
	if err != nil {
		return nil, &net.OpError{
			Op:   "dial",
			Net:  network,
			Addr: remAddr,
			Err:  err,
		}
	}

	locAddr := &net.UDPAddr{
		IP:   srcIP,
		Port: port,
	}

	conn, err := newUDPConn(locAddr, v)
	if err != nil {
		return nil, err
	}

	conn.remAddr = remAddr

	err = v.udpConns.insert(conn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (v *vNet) write(c Chunk) error {
	if c.Network() == "udp" {
		if udp, ok := c.(*chunkUDP); ok {
			if c.getDestinationIP().IsLoopback() {
				if conn, ok := v.udpConns.find(udp.DestinationAddr()); ok {
					select {
					case conn.readCh <- udp:
					default:
					}
				}
				return nil
			}
		} else {
			return fmt.Errorf("unexpected type-switch failure")
		}
	}

	if v.router == nil {
		return fmt.Errorf("no router linked")
	}

	v.router.push(c)
	return nil
}

func (v *vNet) onClosed(addr net.Addr) {
	if addr.Network() == "udp" {
		v.udpConns.delete(addr)
	}
}

// This method determines the srcIP based on the dstIP when locIP
// is any IP address ("0.0.0.0" or "::"). If locIP is a non-any addr,
// this method simply returns locIP.
func (v *vNet) determineSourceIP(locIP, dstIP net.IP) net.IP {
	if !isAnyIP(locIP) {
		return locIP
	}

	var srcIP net.IP

	if dstIP.IsLoopback() {
		srcIP = net.ParseIP("127.0.0.1")
	} else {
		ifc, err2 := v._getInterface("eth0")
		if err2 != nil {
			return nil
		}

		addrs, err2 := ifc.Addrs()
		if err2 != nil {
			return nil
		}

		if len(addrs) == 0 {
			return nil
		}

		isIPv4 := (locIP.To4() != nil)

		for _, addr := range addrs {
			ip := addr.(*net.IPNet).IP
			if isIPv4 {
				if ip.To4() != nil {
					srcIP = ip
					break
				}
			} else {
				if ip.To4() == nil {
					srcIP = ip
					break
				}
			}
		}
	}

	return srcIP
}

// caller must hold the mutex
func (v *vNet) hasIPAddr(ip net.IP) bool {
	for _, ifc := range v.interfaces {
		if addrs, err := ifc.Addrs(); err == nil {
			for _, addr := range addrs {
				var locIP net.IP
				if ipNet, ok := addr.(*net.IPNet); ok {
					locIP = ipNet.IP
				} else if ipAddr, ok := addr.(*net.IPAddr); ok {
					locIP = ipAddr.IP
				} else {
					continue
				}

				switch ip.String() {
				case "0.0.0.0":
					if locIP.To4() != nil {
						return true
					}
				case "::":
					if locIP.To4() == nil {
						return true
					}
				default:
					if locIP.Equal(ip) {
						return true
					}
				}
			}
		}
	}

	return false
}

func (v *vNet) allocateLocalAddr(ip net.IP, port int) error {
	var ips []net.IP

	switch ip.String() {
	case "0.0.0.0":
		ips = v.getAllIPAddrs(false)
	case "::":
		ips = v.getAllIPAddrs(true)
	default:
		if v.hasIPAddr(ip) {
			ips = []net.IP{ip}
		}
	}

	if len(ips) == 0 {
		return fmt.Errorf("bind failed for %s", ip.String())
	}

	for _, ip2 := range ips {
		addr := &net.UDPAddr{
			IP:   ip2,
			Port: port,
		}
		if _, ok := v.udpConns.find(addr); ok {
			return &net.OpError{
				Op:   "bind",
				Net:  "udp",
				Addr: addr,
				Err:  fmt.Errorf("bind: address already in use"),
			}
		}
	}

	return nil
}

func (v *vNet) assignPort(ip net.IP, start, end int) (int, error) {
	// choose randomly from the range between start and end (inclusive)
	if end < start {
		return -1, fmt.Errorf("end port is less than the start")
	}

	space := end + 1 - start
	offset := rand.Intn(space)
	for i := 0; i < space; i++ {
		port := ((offset + i) % space) + start

		err := v.allocateLocalAddr(ip, port)
		if err == nil {
			return port, nil
		}
	}

	return -1, fmt.Errorf("port space exhausted")
}

// NetConfig is a bag of configuration parameters passed to NewNet().
type NetConfig struct {
	// StaticIP is a static IP address to be assigned for this Net. If nil,
	// the router will automatically assign an IP address.
	StaticIP string
}

// Net represents a local network stack euivalent to a set of layers from NIC
// up to the transport (UDP / TCP) layer.
type Net struct {
	v   *vNet
	ifs []*Interface
}

// NewNet creates an instance of Net.
// If config is nil, the virtual network is disabled. (uses corresponding
// net.Xxxx() operations.
// By design, it always have lo0 and eth0 interfaces.
// The lo0 has the address 127.0.0.1 assigned by default.
// IP address for eth0 will be assigned when this Net is added to a router.
func NewNet(config *NetConfig) *Net {
	if config == nil {
		ifs := []*Interface{}
		if orgIfs, err := net.Interfaces(); err == nil {
			for _, orgIfc := range orgIfs {
				ifc := NewInterface(orgIfc)
				if addrs, err := orgIfc.Addrs(); err == nil {
					for _, addr := range addrs {
						ifc.AddAddr(addr)
					}
				}

				ifs = append(ifs, ifc)
			}
		}

		return &Net{ifs: ifs}
	}

	lo0 := NewInterface(net.Interface{
		Index:        1,
		MTU:          16384,
		Name:         "lo0",
		HardwareAddr: nil,
		Flags:        net.FlagUp | net.FlagLoopback | net.FlagMulticast,
	})
	lo0.AddAddr(&net.IPNet{
		IP:   net.ParseIP("127.0.0.1"),
		Mask: net.CIDRMask(8, 32),
	})

	eth0 := NewInterface(net.Interface{
		Index:        2,
		MTU:          1500,
		Name:         "eth0",
		HardwareAddr: newMACAddress(),
		Flags:        net.FlagUp | net.FlagMulticast,
	})

	v := &vNet{
		interfaces: []*Interface{lo0, eth0},
		staticIP:   net.ParseIP(config.StaticIP),
		udpConns:   newUDPConnMap(),
	}

	return &Net{
		v: v,
	}
}

// Interfaces returns a list of the system's network interfaces.
func (n *Net) Interfaces() ([]*Interface, error) {
	if n.v == nil {
		return n.ifs, nil
	}

	return n.v.getInterfaces()
}

// InterfaceByName returns the interface specified by name.
func (n *Net) InterfaceByName(name string) (*Interface, error) {
	if n.v == nil {
		for _, ifc := range n.ifs {
			if ifc.Name == name {
				return ifc, nil
			}
		}

		return nil, fmt.Errorf("interface %s not found", name)
	}

	return n.v.getInterface(name)
}

// ListenPacket announces on the local network address.
func (n *Net) ListenPacket(network string, address string) (net.PacketConn, error) {
	if n.v == nil {
		return net.ListenPacket(network, address)
	}

	return n.v.listenPacket(network, address)
}

// ListenUDP acts like ListenPacket for UDP networks.
func (n *Net) ListenUDP(network string, locAddr *net.UDPAddr) (UDPPacketConn, error) {
	if n.v == nil {
		return net.ListenUDP(network, locAddr)
	}

	return n.v.listenUDP(network, locAddr)
}

// Dial connects to the address on the named network.
func (n *Net) Dial(network, address string) (net.Conn, error) {
	if n.v == nil {
		return net.Dial(network, address)
	}

	return n.v.dial(network, address)
}

// CreateDialer creates an instance of vnet.Dialer
func (n *Net) CreateDialer(dialer *net.Dialer) Dialer {
	if n.v == nil {
		return &vDialer{
			dialer: dialer,
		}
	}

	return &vDialer{
		dialer: dialer,
		v:      n.v,
	}
}

func (n *Net) getInterface(ifName string) (*Interface, error) {
	if n.v == nil {
		return nil, fmt.Errorf("vnet is not enabled")
	}

	return n.v.getInterface(ifName)
}

func (n *Net) setRouter(r *Router) error {
	if n.v == nil {
		return fmt.Errorf("vnet is not enabled")
	}

	return n.v.setRouter(r)
}

func (n *Net) onInboundChunk(c Chunk) {
	if n.v == nil {
		return
	}

	n.v.onInboundChunk(c)
}

func (n *Net) getStaticIP() net.IP {
	if n.v == nil {
		return nil
	}

	return n.v.staticIP
}

// Dialer is identical to net.Dialer excepts that its methods
// (Dial, DialContext) are overridden to use virtual network.
// Use vnet.CreateDialer() to create an instance of this Dialer.
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

type vDialer struct {
	dialer *net.Dialer
	v      *vNet
}

func (d *vDialer) Dial(network, address string) (net.Conn, error) {
	if d.v == nil {
		return d.dialer.Dial(network, address)
	}

	return d.v.dial(network, address)
}
