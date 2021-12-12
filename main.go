package main

import (
	_ "github.com/ca17/datahub/plugin/datahub"
	_ "github.com/ca17/geoforward/plugin/geoforward"
	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/core/plugin"
	"github.com/coredns/coredns/coremain"
)

func index(slice []string, item string) int {
	for i := range slice {
		if slice[i] == item {
			return i
		}
	}
	return -1
}

func main() {
	// insert dnssrc before forward
	idx2 := index(dnsserver.Directives, "geoip")
	dnsserver.Directives = append(dnsserver.Directives[:idx2], append([]string{"datahub"}, dnsserver.Directives[idx2:]...)...)
	idx := index(dnsserver.Directives, "forward")
	dnsserver.Directives = append(dnsserver.Directives[:idx], append([]string{"geoforward"}, dnsserver.Directives[idx:]...)...)
	coremain.Run()
}
