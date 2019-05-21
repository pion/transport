package vnet

import (
	"fmt"
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

	if err := r.addHost("localhost", "127.0.0.1"); err != nil {
		r.log.Warn("failed to add localhost to resolver")
	}
	return r
}

func (r *resolver) setParent(parent *resolver) {
	r.parent = parent
}

func (r *resolver) addHost(name string, ipAddr string) error {
	if len(name) == 0 {
		return fmt.Errorf("host name must not be empty")
	}
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("failed to parse IP address \"%s\"", ipAddr)
	}
	r.hosts[name] = ip
	return nil
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
