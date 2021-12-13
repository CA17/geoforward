/*
 * Created Feb 23, 2020
 */

package geoforward

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/miekg/dns"
)

type reloadableUpstream struct {
	// Flag indicate match any request, i.e. the root zone "."
	matchGeositeTags    []string
	nonMatchGeositeTags []string
	inline              domainSet
	ignored             domainSet
	*HealthCheck
	// Bootstrap DNS in IP:Port combo
	bootstrap []string
	matchAny  bool
	noIPv6    bool
	debug     bool
}

// reloadableUpstream implements Upstream interface

// Check if given name in upstream name list
// `name' is lower cased and without trailing dot(except for root zone)
func (u *reloadableUpstream) Match(name string) bool {
	if u.matchAny {
		if !plugin.Name(".").Matches(name) {
			panic(fmt.Sprintf("Why %q doesn't match %q?!", name, "."))
		}

		ignored := u.ignored.Match(name)
		if ignored {
			log.Debugf("#0 Skip %q since it's ignored", name)
		}
		return !ignored
	}

	if hubPlugin == nil {
		log.Errorf("hubPlugin not enable")
		return false
	}

	// if u.ignored.Match(name) {
	// 	log.Debugf("#1 Skip %q since it's ignored", name)
	// 	return false
	// }
	//
	// if u.inline.Match(name) {
	// 	return true
	// }

	if len(u.matchGeositeTags) > 0 && hubPlugin.MixMatchTags(u.matchGeositeTags, name) {
		log.Debugf("match %s by tags %s", name, strings.Join(u.matchGeositeTags, " "))
		return true
	}

	if len(u.nonMatchGeositeTags) > 0 {
		if hubPlugin.MixMatchTags(u.nonMatchGeositeTags, name) {
			return false
		}
		log.Debugf("name %s in !%v", name, u.nonMatchGeositeTags)
		return true
	}

	return false
}

func (u *reloadableUpstream) Start() error {
	u.HealthCheck.Start()
	return nil
}

func (u *reloadableUpstream) Stop() error {
	u.HealthCheck.Stop()
	return nil
}

// Parses Caddy config input and return a list of reloadable upstream for this plugin
func NewReloadableUpstreams(c *caddy.Controller) ([]Upstream, error) {
	var ups []Upstream

	for c.Next() {
		u, err := newReloadableUpstream(c)
		if err != nil {
			return nil, err
		}
		ups = append(ups, u)
	}

	if ups == nil {
		panic("Why upstream hosts is nil? it shouldn't happen.")
	}
	return ups, nil
}

// see: healthcheck.go/UpstreamHost.Dial()
func protoToNetwork(proto string) string {
	if proto == "tls" {
		return "tcp-tls"
	}
	return proto
}

func newReloadableUpstream(c *caddy.Controller) (Upstream, error) {
	u := &reloadableUpstream{
		matchGeositeTags:    make([]string, 0),
		nonMatchGeositeTags: make([]string, 0),
		ignored:             make(domainSet),
		inline:              make(domainSet),
		HealthCheck: &HealthCheck{
			stop:          make(chan struct{}),
			maxFails:      defaultMaxFails,
			checkInterval: defaultHcInterval,
			transport: &Transport{
				expire:           defaultConnExpire,
				tlsConfig:        new(tls.Config),
				recursionDesired: true,
			},
		},
	}

	if err := parseFrom(c, u); err != nil {
		return nil, err
	}

	for c.NextBlock() {
		if err := parseBlock(c, u); err != nil {
			return nil, err
		}
	}

	if u.hosts == nil {
		return nil, c.Errf("missing mandatory property: %q", "to")
	}
	for _, host := range u.hosts {
		addr, tlsServerName := SplitByByte(host.addr, '@')
		host.addr = addr

		host.transport = newTransport()
		// Inherit from global transport settings
		host.transport.recursionDesired = u.transport.recursionDesired
		host.transport.expire = u.transport.expire
		if host.proto == transport.TLS {
			// Deep copy
			host.transport.tlsConfig = new(tls.Config)
			host.transport.tlsConfig.Certificates = u.transport.tlsConfig.Certificates
			host.transport.tlsConfig.RootCAs = u.transport.tlsConfig.RootCAs
			// Don't set TLS server name if addr host part is already a domain name
			if hostPortIsIpPort(addr) {
				host.transport.tlsConfig.ServerName = u.transport.tlsConfig.ServerName
			}

			// TLS server name in tls:// takes precedence over the global one(if any)
			if len(tlsServerName) != 0 {
				tlsServerName = tlsServerName[1:]
				serverName, ok := stringToDomain(tlsServerName)
				if !ok {
					return nil, c.Errf("invalid TLS server name %q", tlsServerName)
				}
				host.transport.tlsConfig.ServerName = serverName
			}
		}

		network := protoToNetwork(host.proto)
		if network == "dns" {
			// Use classic DNS protocol for health checking
			network = "udp"
		}
		host.c = &dns.Client{
			Net:       network,
			TLSConfig: host.transport.tlsConfig,
			Timeout:   defaultHcTimeout,
		}
		host.InitDOH(u)
	}

	if u.matchAny {
		if len(u.inline) != 0 {
			return nil, c.Errf("INLINE %q is forbidden since %q will match all requests", u.inline, ".")
		}
	}

	if len(u.inline) != 0 {
		log.Infof("inline: %v", u.inline)
	}

	return u, nil
}

func parseFrom(c *caddy.Controller, u *reloadableUpstream) error {
	forms := c.RemainingArgs()
	n := len(forms)
	if n == 0 {
		return c.ArgErr()
	}

	if n == 1 && forms[0] == "." {
		u.matchAny = true
		log.Infof("Match any")
		return nil
	}
	for _, form := range forms {
		if form[0] == '!' {
			u.nonMatchGeositeTags = append(u.nonMatchGeositeTags, form[1:])
		} else {
			u.matchGeositeTags = append(u.matchGeositeTags, form)
		}
	}

	log.Infof("FROM...: match: %v nonmatch: %v", u.matchGeositeTags, u.nonMatchGeositeTags)
	return nil
}

func parseBlock(c *caddy.Controller, u *reloadableUpstream) error {
	switch dir := c.Val(); dir {
	case "except":
		// Multiple "except"s will be merged together
		args := c.RemainingArgs()
		if len(args) == 0 {
			return c.ArgErr()
		}
		for _, name := range args {
			if !u.ignored.Add(name) {
				log.Warningf("%q isn't a domain name", name)
			}
		}
		log.Infof("%v: %v", dir, u.ignored)
	case "spray":
		if len(c.RemainingArgs()) != 0 {
			return c.ArgErr()
		}
		u.spray = &Spray{}
		log.Infof("%v: enabled", dir)
	case "policy":
		arr := c.RemainingArgs()
		if len(arr) != 1 {
			return c.ArgErr()
		}
		policy, ok := SupportedPolicies[arr[0]]
		if !ok {
			return c.Errf("unknown policy: %q", arr[0])
		}
		u.policy = policy
		log.Infof("%v: %v", dir, arr[0])
	case "max_fails":
		n, err := parseInt32(c)
		if err != nil {
			return err
		}
		u.maxFails = n
		log.Infof("%v: %v", dir, n)
	case "health_check":
		args := c.RemainingArgs()
		n := len(args)
		if n != 1 && n != 2 {
			return c.ArgErr()
		}
		dur, err := parseDuration0(dir, args[0])
		if err != nil {
			return c.Err(err.Error())
		}
		if dur < minHcInterval && dur != 0 {
			return c.Errf("%v: minimal interval is %v", dir, minHcInterval)
		}
		if n == 2 && args[1] != "no_rec" {
			return c.Errf("%v: unknown option: %v", dir, args[1])
		}
		u.checkInterval = dur
		u.transport.recursionDesired = n == 1
		log.Infof("%v: %v %v", dir, u.checkInterval, u.transport.recursionDesired)
	case "to":
		// Multiple "to"s will be merged together
		if err := parseTo(c, u); err != nil {
			return err
		}
	case "expire":
		dur, err := parseDuration(c)
		if err != nil {
			return err
		}
		if dur < minExpireInterval && dur != 0 {
			return c.Errf("%v: minimal interval is %v", dir, minExpireInterval)
		}
		u.transport.expire = dur
		log.Infof("%v: %v", dir, dur)
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}
		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		// Merge server name if tls_servername set previously
		tlsConfig.ServerName = u.transport.tlsConfig.ServerName
		u.transport.tlsConfig = tlsConfig
		log.Infof("%v: %v", dir, args)
	case "tls_servername":
		args := c.RemainingArgs()
		if len(args) != 1 {
			return c.ArgErr()
		}
		serverName, ok := stringToDomain(args[0])
		if !ok {
			return c.Errf("%v: %q isn't a valid domain name", dir, args[0])
		}
		u.transport.tlsConfig.ServerName = serverName
		log.Infof("%v: %v", dir, serverName)
	case "match_client":
		args := c.RemainingArgs()
		log.Info(args)
		if c.NextBlock() {
			err := parseSubBlock(c, u)
			if err != nil {
				return err
			}
		}
	case "no_ipv6":
		args := c.RemainingArgs()
		if len(args) != 0 {
			return c.ArgErr()
		}
		u.noIPv6 = true
		log.Infof("%v: %v", dir, u.noIPv6)
	case "debug":
		u.debug = true
	default:
		return c.Errf("unknown property: %q", dir)
	}
	return nil
}

func parseSubBlock(c *caddy.Controller, u *reloadableUpstream) error {
	for c.Next() {
		switch dir := c.Val(); dir {
		case "force_ecs":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return c.ArgErr()
			}
			log.Info(args)
		default:

		}
	}
	return nil
}

// Return a non-negative int32
// see: https://golang.org/pkg/builtin/#int
func parseInt32(c *caddy.Controller) (int32, error) {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) != 1 {
		return 0, c.ArgErr()
	}

	n, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, err
	}

	// In case of n is 64-bit
	if n < 0 || n > 0x7fffffff {
		return 0, c.Errf("%v: value %v of out of non-negative int32 range", dir, n)
	}

	return int32(n), nil
}

func parseDuration0(dir, arg string) (time.Duration, error) {
	duration, err := time.ParseDuration(arg)
	if err != nil {
		return 0, err
	}

	if duration < 0 {
		return 0, errors.New(fmt.Sprintf("%v: negative time duration %v", dir, arg))
	}
	return duration, nil
}

// Return a non-negative time.Duration and an error(if any)
func parseDuration(c *caddy.Controller) (time.Duration, error) {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) != 1 {
		return 0, c.ArgErr()
	}
	dur, err := parseDuration0(dir, args[0])
	if err == nil {
		return dur, nil
	}
	return dur, c.Err(err.Error())
}

func parseTo(c *caddy.Controller, u *reloadableUpstream) error {
	args := c.RemainingArgs()
	if len(args) == 0 {
		return c.ArgErr()
	}

	toHosts, err := HostPort(args)
	if err != nil {
		return err
	}

	for _, host := range toHosts {
		trans, addr := SplitTransportHost(host)
		log.Infof("Transport: %v Address: %v", trans, addr)

		uh := &UpstreamHost{
			proto: trans,
			// Not an error, host and tls server name will be separated later
			addr:     addr,
			downFunc: checkDownFunc(u),
		}
		u.hosts = append(u.hosts, uh)

		log.Infof("Upstream: %v", uh)
	}

	return nil
}

const (
	defaultMaxFails = 3

	defaultPathReloadInterval = 2 * time.Second
	defaultUrlReloadInterval  = 30 * time.Minute
	defaultUrlReadTimeout     = 15 * time.Second

	defaultHcInterval = 2000 * time.Millisecond
	defaultHcTimeout  = 5000 * time.Millisecond
)

const (
	minPathReloadInterval = 1 * time.Second
	minUrlReloadInterval  = 15 * time.Second
	minUrlReadTimeout     = 3 * time.Second

	minHcInterval     = 1 * time.Second
	minExpireInterval = 1 * time.Second
)
