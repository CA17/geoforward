package metadnsq

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin(pluginName)

type MetaForward struct {
	Next      plugin.Handler
	Upstreams *[]Upstream
}

// Upstream manages a pool of proxy upstream hosts
// see: github.com/coredns/proxy#proxy.go
type Upstream interface {
	// Check if given domain name should be routed to this upstream zone
	Match(name string) bool
	// Select an upstream host to be routed to, nil if no available host
	Select() *UpstreamHost

	// Exchanger returns the exchanger to be used for this upstream
	// Exchanger() interface{}
	// Send question to upstream host and await for response
	// Exchange(ctx context.Context, state request.Request) (*dns.Msg, error)

	Start() error
	Stop() error
}

func (r *MetaForward) OnStartup() error {
	for _, up := range *r.Upstreams {
		if err := up.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (r *MetaForward) OnShutdown() error {
	for _, up := range *r.Upstreams {
		if err := up.Stop(); err != nil {
			return err
		}
	}

	return nil
}

func (r *MetaForward) ServeDNS(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) (int, error) {
	state := &request.Request{W: w, Req: req}
	name := state.Name()

	server := metrics.WithServer(ctx)
	upstream0, t := r.match(server, name)
	if upstream0 == nil {
		log.Debugf("%q not found in name list, t: %v", name, t)
		return plugin.NextOrFailure(r.Name(), r.Next, ctx, w, req)
	}
	upstream := upstream0.(*reloadableUpstream)
	var rwrite = NewResponseReverter(w)

	// 请求参数匹配处理
	qmatcher, rcode := r.matchQuery(upstream, state, rwrite)
	if rcode != dns.RcodeSuccess {
		return rcode, nil
	}

	log.Debugf("%q in name list, t: %v", name, t)

	var reply *dns.Msg
	var upstreamErr error
	deadline := time.Now().Add(defaultTimeout)
	for time.Now().Before(deadline) {
		start := time.Now()

		var host *UpstreamHost

		if qmatcher != nil && qmatcher.to != "" {
			host = upstream.SelectByTag(qmatcher.to)
		}

		if host == nil {
			host = upstream.Select()
		}

		if host == nil {
			log.Debug(errNoHealthy)
			return dns.RcodeServerFailure, errNoHealthy
		}

		log.Debugf("Upstream host %v is selected", host.Name())

		for {
			t := time.Now()
			reply, upstreamErr = host.Exchange(ctx, state, upstream.bootstrap, upstream.noIPv6)
			log.Debugf("rtt: %v", time.Since(t))
			if upstreamErr == errCachedConnClosed {
				// [sic] Remote side closed conn, can only happen with TCP.
				// Retry for another connection
				log.Debugf("%v: %v", upstreamErr, host.Name())
				continue
			}
			break
		}

		if upstreamErr != nil {
			if upstream.maxFails != 0 {
				log.Warningf("Exchange() failed  error: %v", upstreamErr)
				healthCheck(upstream, host)
			}
			continue
		}

		if !state.Match(reply) {
			debug.Hexdumpf(reply, "Wrong reply  id: %v, qname: %v qtype: %v", reply.Id, state.QName(), state.QType())

			formerr := new(dns.Msg)
			formerr.SetRcode(state.Req, dns.RcodeFormatError)
			_ = rwrite.WriteMsg(formerr)
			return dns.RcodeSuccess, nil
		}

		// 响应参数匹配处理
		rcode := r.matchAnwser(upstream, state, reply)
		if rcode != dns.RcodeSuccess {
			return rcode, nil
		}
		_ = rwrite.WriteMsg(reply)

		RequestDuration.WithLabelValues(server, host.Name()).Observe(float64(time.Since(start).Milliseconds()))
		RequestCount.WithLabelValues(server, host.Name()).Inc()

		rc, ok := dns.RcodeToString[reply.Rcode]
		if !ok {
			rc = strconv.Itoa(reply.Rcode)
		}
		RcodeCount.WithLabelValues(server, host.Name(), rc).Inc()
		return dns.RcodeSuccess, nil
	}

	if upstreamErr == nil {
		panic("Why upstreamErr is nil?! Are you in a debugger or your machine running slow?")
	}
	return dns.RcodeServerFailure, upstreamErr
}

func (r *MetaForward) matchQuery(upstream *reloadableUpstream, state *request.Request, rwrite *ResponseReverter) (*subMatcher, int) {
	var name, ip = state.Name(), state.IP()
	if len(name) > 1 {
		name = removeTrailingDot(name)
	}
	qmatcher := upstream.subMatchers.matchQuery(name)
	if qmatcher == nil {
		qmatcher = upstream.subMatchers.matchClientIp(ip)
	}
	if qmatcher != nil {

		if upstream.debug {
			log.Infof("matchQuery %s", qmatcher.String())
		}

		if qmatcher.notify != "" {
			hubPlugin.NotifyMessage(qmatcher.notify, state)
		}

		qmatcher.ipset.ForEach(func(sname string) {
			ipsetAddIPByName(upstream, state.Req, sname)
		})

		// 匹配 NXDOMAIN
		if qmatcher.nxdomain {
			return nil, dns.RcodeNameError
		}

		// set ecs
		if qmatcher.forceEcs != "" {
			ecsip := hubPlugin.MatchEcs(qmatcher.forceEcs, ip)
			if ecsip != nil {
				var qHasECS = getMsgECS(state.Req) != nil
				var ecs *dns.EDNS0_SUBNET
				if ip4 := ecsip.To4(); ip4 != nil { // is ipv4
					ecs = newEDNS0Subnet(ip4, 24, false)
				} else {
					if ip6 := ecsip.To16(); ip6 != nil { // is ipv6
						ecs = newEDNS0Subnet(ip6, 48, true)
					}
				}
				// 强制设置 ECS， 如果请求本身没有 ECS， 那么响应中必须清除 ECS
				if ecs != nil {
					setECS(state.Req, ecs)
					rwrite.removeEcs = qHasECS
				}
			}
		}
	}
	return qmatcher, 0
}

func (r *MetaForward) matchAnwser(upstream *reloadableUpstream, state *request.Request, reply *dns.Msg) int {
	for _, rr := range reply.Answer {
		var rmatcher *subMatcher
		switch rr.(type) {
		case *dns.A:
			rra := rr.(*dns.A)
			rmatcher = upstream.subMatchers.matchAnwserIp(rra.A.String())
		case *dns.AAAA:
			rra := rr.(*dns.AAAA)
			rmatcher = upstream.subMatchers.matchAnwserIp(rra.AAAA.String())
		case *dns.CNAME:
			rra := rr.(*dns.CNAME)
			name := rra.Target
			if len(name) > 1 {
				name = removeTrailingDot(name)
			}
			rmatcher = upstream.subMatchers.matchAnwser(rra.Target)
		}

		if rmatcher == nil {
			continue
		}

		if upstream.debug {
			log.Infof("matchAnwser %s", rmatcher.String())
		}

		if rmatcher.notify != "" {
			hubPlugin.NotifyMessage(rmatcher.notify, state)
		}

		rmatcher.ipset.ForEach(func(sname string) {
			ipsetAddIPByName(upstream, reply, sname)
		})

		if rmatcher != nil && rmatcher.nxdomain {
			return dns.RcodeNameError
		}
	}
	return dns.RcodeSuccess
}

func healthCheck(r *reloadableUpstream, uh *UpstreamHost) {
	// Skip unnecessary health checking
	if r.checkInterval == 0 || r.maxFails == 0 {
		return
	}

	failTimeout := defaultFailTimeout
	fails := atomic.AddInt32(&uh.fails, 1)
	go func(uh *UpstreamHost) {
		time.Sleep(failTimeout)
		// Failure count may go negative here, should be rectified by HC eventually
		atomic.AddInt32(&uh.fails, -1)
		// Kick off health check on every failureCheck failure
		if fails%failureCheck == 0 {
			_ = uh.Check()
		}
	}(uh)
}

func (r *MetaForward) Name() string { return pluginName }

func (r *MetaForward) match(server, name string) (Upstream, time.Duration) {
	t1 := time.Now()

	if r.Upstreams == nil {
		panic("Why MetaForward.Upstreams is nil?!")
	}

	if len(name) > 1 {
		name = removeTrailingDot(name)
	}

	for _, up := range *r.Upstreams {
		// For maximum performance, we search the first matched item and return directly
		// Unlike proxy plugin, which try to find longest match
		if up.Match(name) {
			t2 := time.Since(t1)
			NameLookupDuration.WithLabelValues(server, "1").Observe(float64(t2.Milliseconds()))
			return up, t2
		}
	}

	t2 := time.Since(t1)
	NameLookupDuration.WithLabelValues(server, "0").Observe(float64(t2.Milliseconds()))
	return nil, t2
}

var (
	errNoHealthy        = errors.New("no healthy upstream host")
	errCachedConnClosed = errors.New("cached connection was closed by peer")
)

const (
	defaultTimeout     = 15 * time.Second
	defaultFailTimeout = 2000 * time.Millisecond
	failureCheck       = 3
)
