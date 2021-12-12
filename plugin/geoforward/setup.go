package geoforward

import (
	"github.com/ca17/datahub/plugin/datahub"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register(pluginName, setup) }

var hubPlugin *datahub.Datahub

func setup(c *caddy.Controller) error {
	log.Infof("Initializing, version %v, HEAD %v", pluginVersion, pluginHeadCommit)

	ups, err := NewReloadableUpstreams(c)
	if err != nil {
		return PluginError(err)
	}

	r := &GeoForward{Upstreams: &ups}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		r.Next = next
		return r
	})

	c.OnStartup(func() error {
		if dhub := dnsserver.GetConfig(c).Handler("datahub"); dhub != nil {
			if hp, ok := dhub.(*datahub.Datahub); ok {
				hubPlugin = hp
			}
		}
		return r.OnStartup()
	})

	c.OnShutdown(func() error {
		return r.OnShutdown()
	})

	return nil
}
