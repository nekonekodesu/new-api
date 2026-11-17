package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const customDNSQueryTimeout = 5 * time.Second

func init() {
	common.LookupIPFunc = LookupIPWithConfiguredDNS
}

type outboundDNSResolver struct {
	getServers     func() []string
	systemResolver ssrfResolver
	dnsDialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

var defaultOutboundDNSResolver = &outboundDNSResolver{
	getServers:     system_setting.ConfiguredDNSServers,
	systemResolver: net.DefaultResolver,
}

func newResolverForDNSServer(serverAddr string, dnsDialContext func(ctx context.Context, network, address string) (net.Conn, error)) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if dnsDialContext != nil {
				return dnsDialContext(ctx, network, serverAddr)
			}
			dialer := &net.Dialer{Timeout: customDNSQueryTimeout}
			return dialer.DialContext(ctx, network, serverAddr)
		},
	}
}

func (r *outboundDNSResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}

	var servers []string
	if r.getServers != nil {
		servers = r.getServers()
	}

	var lastErr error
	for _, server := range servers {
		if server == "" {
			continue
		}
		queryCtx, cancel := context.WithTimeout(ctx, customDNSQueryTimeout)
		resolver := newResolverForDNSServer(server, r.dnsDialContext)
		addrs, err := resolver.LookupIPAddr(queryCtx, host)
		cancel()
		if err == nil && len(addrs) > 0 {
			return addrs, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	sysResolver := r.systemResolver
	if sysResolver == nil {
		sysResolver = net.DefaultResolver
	}
	addrs, err := sysResolver.LookupIPAddr(ctx, host)
	if err == nil && len(addrs) > 0 {
		return addrs, nil
	}
	if err != nil {
		return nil, err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("DNS resolution for %s returned no IP addresses", host)
}

// LookupIPWithConfiguredDNS resolves host using configured primary and fallback
// DNS servers before falling back to the system resolver.
func LookupIPWithConfiguredDNS(host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addrs, err := defaultOutboundDNSResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if addr.IP != nil {
			ips = append(ips, addr.IP)
		}
	}
	return ips, nil
}

func dialContextWithConfiguredDNS(
	baseDialContext func(ctx context.Context, network, address string) (net.Conn, error),
	resolver ssrfResolver,
) func(ctx context.Context, network, address string) (net.Conn, error) {
	if baseDialContext == nil {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		baseDialContext = dialer.DialContext
	}
	if resolver == nil {
		resolver = defaultOutboundDNSResolver
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if len(system_setting.ConfiguredDNSServers()) == 0 && resolver == defaultOutboundDNSResolver {
			return baseDialContext(ctx, network, address)
		}

		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return baseDialContext(ctx, network, address)
		}
		if net.ParseIP(host) != nil {
			return baseDialContext(ctx, network, address)
		}

		addrs, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}

		var lastDialErr error
		for _, ipAddr := range addrs {
			ip := ipAddr.IP
			if ip == nil || !networkAllowsIP(network, ip) {
				continue
			}
			conn, dialErr := baseDialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastDialErr = dialErr
		}
		if lastDialErr != nil {
			return nil, lastDialErr
		}
		return nil, fmt.Errorf("DNS resolution for %s returned no usable IP addresses", host)
	}
}
