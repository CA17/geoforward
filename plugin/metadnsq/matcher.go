package metadnsq

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/c-robinson/iplib"
	"github.com/ca17/datahub/plugin/pkg/netutils"
	"github.com/ca17/datahub/plugin/pkg/stringset"
)

type subMatcher struct {
	isValid      bool
	name         string
	to           string
	clientIps    *stringset.StringSet
	anwserIps    *stringset.StringSet
	queryNames   *stringset.StringSet
	anwserCNames *stringset.StringSet
	forceEcs     string
	notify       string
	nxdomain     bool
	ipset        *stringset.StringSet
}

func newSubMatcher() *subMatcher {
	return &subMatcher{
		clientIps:    stringset.New(),
		anwserIps:    stringset.New(),
		queryNames:   stringset.New(),
		anwserCNames: stringset.New(),
		ipset:        stringset.New(),
	}
}

func (m *subMatcher) String() string {
	return fmt.Sprintf("submatch >> to:%s clientIps:%s qname:%s anwserIps:%s cname:%s notify:%s ecs:%s ipset:%s nxdomain:%s",
		m.to,
		m.clientIps,
		m.queryNames.String(),
		m.anwserIps.String(),
		m.anwserCNames.String(),
		m.forceEcs,
		m.notify,
		m.ipset.String(),
		strconv.FormatBool(m.nxdomain),
	)
}

func (m *subMatcher) matchQueryName(name string) bool {
	return hubPlugin.MixMatchTags(m.queryNames.Slice(), name)
}

func (m *subMatcher) matchAnwserCname(name string) bool {
	return hubPlugin.MixMatchTags(m.anwserCNames.Slice(), name)
}

func (m *subMatcher) matchClientIp(ns iplib.Net) bool {
	return m.clientIps.MatchFnFirst(func(src string) bool {
		return hubPlugin.MixMatchNet(src, ns)
	})
}

func (m *subMatcher) matchAnwserIp(ns iplib.Net) bool {
	return m.anwserIps.MatchFnFirst(func(src string) bool {
		return hubPlugin.MixMatchNet(src, ns)
	})
}

type subMatchers struct {
	sync.RWMutex
	matchers []*subMatcher
}

func newSubMatchers() *subMatchers {
	return &subMatchers{matchers: make([]*subMatcher, 0)}
}

func (ms *subMatchers) addSubMatcher(m *subMatcher) {
	ms.matchers = append(ms.matchers, m)
}

func (ms *subMatchers) matchQuery(name string) *subMatcher {
	ms.RLock()
	defer ms.RUnlock()
	for _, matcher := range ms.matchers {
		if matcher.matchQueryName(name) {
			return matcher
		}
	}
	return nil
}

func (ms *subMatchers) matchAnwser(name string) *subMatcher {
	ms.RLock()
	defer ms.RUnlock()
	for _, matcher := range ms.matchers {
		if matcher.matchAnwserCname(name) {
			return matcher
		}
	}
	return nil
}

func (ms *subMatchers) matchClientIp(ip string) *subMatcher {
	ns, err := netutils.ParseIpNet(ip)
	if err != nil {
		return nil
	}
	ms.RLock()
	defer ms.RUnlock()
	for _, matcher := range ms.matchers {
		if matcher.matchClientIp(ns) {
			return matcher
		}
	}
	return nil
}

func (ms *subMatchers) matchAnwserIp(ip string) *subMatcher {
	ns, err := netutils.ParseIpNet(ip)
	if err != nil {
		return nil
	}
	ms.RLock()
	defer ms.RUnlock()
	for _, matcher := range ms.matchers {
		if matcher.matchAnwserIp(ns) {
			return matcher
		}
	}
	return nil
}
