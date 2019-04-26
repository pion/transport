package vnet

import (
	"net"
	"testing"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestResolver(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("Standalone", func(t *testing.T) {
		r := newResolver(&resolverConfig{
			LoggerFactory: loggerFactory,
		})

		// should have localhost by default
		name := "localhost"
		ip := net.ParseIP("127.0.0.1")

		resolved, err := r.lookUp(name)
		assert.Nil(t, err, "should succeeed")
		assert.True(t, resolved.Equal(ip), "should match")

		name = "abc.com"
		ip = net.ParseIP("1.2.3.4")
		log.Debugf("adding %s %v", name, ip)

		r.addHost(name, ip)

		resolved, err = r.lookUp(name)
		assert.Nil(t, err, "should succeeed")
		assert.True(t, resolved.Equal(ip), "should match")
	})

	t.Run("Cascaded", func(t *testing.T) {
		r0 := newResolver(&resolverConfig{
			LoggerFactory: loggerFactory,
		})
		name0 := "abc.com"
		ip0 := net.ParseIP("1.2.3.4")
		r0.addHost(name0, ip0)

		r1 := newResolver(&resolverConfig{
			LoggerFactory: loggerFactory,
		})
		name1 := "myserver.local"
		ip1 := net.ParseIP("10.1.2.5")
		r1.addHost(name1, ip1)
		r1.setParent(r0)

		resolved, err := r1.lookUp(name0)
		assert.Nil(t, err, "should succeeed")
		assert.True(t, resolved.Equal(ip0), "should match")

		resolved, err = r1.lookUp(name1)
		assert.Nil(t, err, "should succeeed")
		assert.True(t, resolved.Equal(ip1), "should match")

		// should fail if the name does not exist
		_, err = r1.lookUp("bad.com")
		assert.NotNil(t, err, "should fail")
	})
}
