//go:build !linux
// +build !linux

package metadnsq

import (
	"runtime"

	"github.com/coredns/caddy"
	"github.com/miekg/dns"
)

var ipsetOnce Once

func ipsetParse(c *caddy.Controller, u *reloadableUpstream) error {
	_ = u
	dir := c.Val()
	// #9 Consume remaining arguments to fix Corefile parse error
	_ = c.RemainingArgs()
	ipsetOnce.Do(func() {
		log.Warningf("%v is not available on %v", dir, runtime.GOOS)
	})
	return nil
}

func ipsetSetup(u *reloadableUpstream) error {
	_ = u
	return nil
}

func ipsetShutdown(u *reloadableUpstream) error {
	_ = u
	return nil
}

func ipsetAddIP(r *reloadableUpstream, reply *dns.Msg) {
	_, _ = r, reply
}

func ipsetAddIPByName(r *reloadableUpstream, reply *dns.Msg, name string) {
	_, _ = r, reply
}
