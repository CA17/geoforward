module github.com/ca17/metadnsq

go 1.16

require (
	github.com/c-robinson/iplib v1.0.3
	github.com/ca17/datahub v0.0.0-20211212121504-b067b4e9ea3b
	github.com/ca17/dnssrc v0.0.2
	github.com/coredns/caddy v1.1.1
	github.com/coredns/coredns v1.8.6
	github.com/m13253/dns-over-https/v2 v2.3.0
	github.com/miekg/dns v1.1.43
	github.com/prometheus/client_golang v1.11.0
	golang.org/x/net v0.0.0-20211201190559-0a0e4e1bb54c
)

replace (
	github.com/ca17/datahub => /Users/wangjuntao/github/coredns-plugins/datahub
)