package vnet

import (
	"net"

	"github.com/pion/logging"
)

type resolverConfig struct {
	LoggerFactory logging.LoggerFactory
}

type resolver struct {
	parent *resolver
	hosts  map[string]net.IP
	log    logging.LeveledLogger
}

func newResolver(config *resolverConfig) *resolver {
	r := &resolver{
		hosts: map[string]net.IP{},
		log:   config.LoggerFactory.NewLogger("vnet"),
	}

	r.addHost("localhost", net.ParseIP("127.0.0.1"))
	return r
}

func (r *resolver) setParent(parent *resolver) {
	r.parent = parent
}

func (r *resolver) addHost(name string, ip net.IP) {
	r.hosts[name] = ip
}

func (r *resolver) lookUp(hostName string) (net.IP, error) {
	if ip, ok := r.hosts[hostName]; ok {
		return ip, nil
	}

	if r.parent != nil {
		return r.parent.lookUp(hostName)
	}

	return nil, &net.DNSError{
		Err:         "host not found",
		Name:        hostName,
		Server:      "vnet resolver",
		IsTimeout:   false,
		IsTemporary: false,
	}
}
