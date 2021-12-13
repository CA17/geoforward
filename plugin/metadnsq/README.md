# dnssrc

*dnssrc* - a CoreDNS forwarding/proxy plugin with the main function of triaging based on client sources

This plugin is based on the `github.com/leiless/dnsredir` extension, many thanks to `dnsredir ` for the inspiration

Most of the configurations are similar except for the client-based source address changes

# Example

    . { 
        # Read address list file，One IP address or CIDR per line
        dnssrc locals.conf {
            expire 30s
            path_reload 3s
            max_fails 0
            health_check 5s
            to 114.114.114.114 223.5.5.5
            policy round_robin
            bootstrap 172.21.66.1
            debug
        }

        # Read url data, Content format as above
        dnssrc https://xxx.xx/xx {
        
        }
    
        dnssrc 172.21.66.137 192.168.0.1/24 {
            expire 1s
            path_reload 3s
            max_fails 0
            health_check 5s
            to json-doh://dns.google/resolve
            to 1.1.1.1
            to 9.9.9.9
            policy round_robin
            bootstrap 172.21.66.1
            debug
        }
    }