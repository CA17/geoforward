module github.com/ca17/metadnsq

go 1.16

require (
	github.com/c-robinson/iplib v1.0.3
	github.com/ca17/datahub v0.0.0-20211223004828-5b73e73d70eb
	github.com/coredns/caddy v1.1.1
	github.com/coredns/coredns v1.8.6
	github.com/digineo/go-ipset/v2 v2.2.1
	github.com/m13253/dns-over-https/v2 v2.3.0
	github.com/miekg/dns v1.1.45
	github.com/prometheus/client_golang v1.11.0
	golang.org/x/net v0.0.0-20211216030914-fe4d6282115f
)

replace github.com/ca17/datahub => ../datahub
