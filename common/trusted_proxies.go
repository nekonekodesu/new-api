package common

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

var cloudflareProxyCIDRs = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

var defaultTrustedProxyCIDRs = append([]string{
	"127.0.0.0/8",
	"::1",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
	"fe80::/10",
}, cloudflareProxyCIDRs...)

type trustedProxyCache struct {
	rawEnv   string
	prefixes []netip.Prefix
}

var (
	cloudflareProxyPrefixes    = mustParseTrustedPrefixes(cloudflareProxyCIDRs)
	defaultTrustedPrefixes     = mustParseTrustedPrefixes(defaultTrustedProxyCIDRs)
	activeTrustedProxyPrefixes atomic.Pointer[trustedProxyCache]
)

// ResolveTrustedProxies parses TRUSTED_PROXIES without applying it to an
// engine. The returned slice can be reused by the outer and plugin engines.
func ResolveTrustedProxies(raw string) (trustedProxies []string, usedDefaults bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), defaultTrustedProxyCIDRs...), true, nil
	}
	if strings.EqualFold(raw, "none") {
		return nil, false, nil
	}

	parts := strings.Split(raw, ",")
	trustedProxies = make([]string, 0, len(parts))
	for _, part := range parts {
		trustedProxy := strings.TrimSpace(part)
		if trustedProxy == "" {
			continue
		}
		if strings.EqualFold(trustedProxy, "none") {
			return nil, false, errors.New("TRUSTED_PROXIES=none must be used alone")
		}
		trustedProxies = append(trustedProxies, trustedProxy)
	}
	if len(trustedProxies) == 0 {
		return nil, false, errors.New("TRUSTED_PROXIES does not contain an IP address or CIDR")
	}
	return trustedProxies, false, nil
}

func ConfigureTrustedProxies(engine *gin.Engine, trustedProxies []string) error {
	if err := engine.SetTrustedProxies(trustedProxies); err != nil {
		return fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}
	prefixes, err := parseTrustedPrefixes(trustedProxies)
	if err == nil {
		activeTrustedProxyPrefixes.Store(&trustedProxyCache{
			rawEnv:   strings.TrimSpace(getTrustedProxiesEnv()),
			prefixes: prefixes,
		})
	}
	return nil
}

// NormalizeForwardedClientIP normalizes load-balancer forwarded IP headers
// (such as host:port entries in X-Forwarded-For / X-Real-IP, or Cloudflare
// CF-Connecting-IP when an intermediate trusted reverse proxy overwrote
// X-Forwarded-For with $remote_addr) so Gin's c.ClientIP() resolves the real
// client IP consistently across logging, rate limiting, and security bans.
// Headers from untrusted direct peers or when TRUSTED_PROXIES=none are never used.
func NormalizeForwardedClientIP(c *gin.Context) {
	_ = resolveAndNormalizeClientIP(c, true)
}

// GetClientIP returns the resolved client IP using new-api's Gin ClientIP /
// TRUSTED_PROXIES pipeline, including multi-hop load balancer and CDN fallback
// normalization when the request arrived from a trusted proxy.
func GetClientIP(c *gin.Context) string {
	return resolveAndNormalizeClientIP(c, false)
}

func resolveAndNormalizeClientIP(c *gin.Context, mutateHeaders bool) string {
	if c == nil || c.Request == nil {
		return ""
	}
	prefixes := currentTrustedProxyPrefixes()
	remoteIP, ok := parseNormalizedAddr(c.RemoteIP())
	if !ok || !containsPrefixAddr(prefixes, remoteIP) {
		if ip, valid := parseNormalizedAddr(c.ClientIP()); valid {
			return ip.String()
		}
		return strings.TrimSpace(c.ClientIP())
	}

	if mutateHeaders {
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			if cleaned, changed := sanitizeForwardedList(xff); changed {
				c.Request.Header.Set("X-Forwarded-For", cleaned)
			}
		}
		if xri := c.GetHeader("X-Real-IP"); xri != "" {
			if addr, valid := parseNormalizedAddr(xri); valid && addr.String() != strings.TrimSpace(xri) {
				c.Request.Header.Set("X-Real-IP", addr.String())
			}
		}
	}

	rawClientIP := strings.TrimSpace(c.ClientIP())
	resolvedIP, resolvedOK := parseNormalizedAddr(rawClientIP)

	// If X-Forwarded-For contained host:port tokens and mutateHeaders was false,
	// evaluate the sanitized X-Forwarded-For chain using the same right-to-left
	// trusted-proxy walk as Gin.
	if (!resolvedOK || containsPrefixAddr(prefixes, resolvedIP)) && c.GetHeader("X-Forwarded-For") != "" {
		if xffIP, valid := walkForwardedChain(c.GetHeader("X-Forwarded-For"), prefixes); valid {
			resolvedIP = xffIP
			resolvedOK = true
			rawClientIP = xffIP.String()
		}
	}

	if resolvedOK && !containsPrefixAddr(prefixes, resolvedIP) {
		return resolvedIP.String()
	}

	// Every hop seen in X-Forwarded-For / RemoteAddr is a trusted proxy.
	// 1. If the outermost proxy hop is an official Cloudflare edge IP (e.g. local
	//    Nginx overwrote X-Forwarded-For with $remote_addr), CF-Connecting-IP is
	//    overwritten by Cloudflare at the edge and carries the real client IP.
	cfUpstream := containsPrefixAddr(cloudflareProxyPrefixes, remoteIP) ||
		(resolvedOK && containsPrefixAddr(cloudflareProxyPrefixes, resolvedIP))
	if !cfUpstream {
		if xri, valid := parseNormalizedAddr(c.GetHeader("X-Real-IP")); valid && containsPrefixAddr(cloudflareProxyPrefixes, xri) {
			cfUpstream = true
		}
	}
	if cfUpstream {
		for _, headerName := range []string{"CF-Connecting-IP", "True-Client-IP"} {
			if cfIP, valid := parseNormalizedAddr(c.GetHeader(headerName)); valid && !containsPrefixAddr(prefixes, cfIP) {
				if mutateHeaders {
					prependForwardedClientIP(c, cfIP.String())
				}
				return cfIP.String()
			}
		}
	}

	// 2. Check X-Real-IP (including host:port form) when it holds a non-proxy client IP.
	if xri, valid := parseNormalizedAddr(c.GetHeader("X-Real-IP")); valid && !containsPrefixAddr(prefixes, xri) {
		if mutateHeaders {
			prependForwardedClientIP(c, xri.String())
		}
		return xri.String()
	}

	// 3. Check standard RFC 7239 Forwarded header right-to-left.
	if fwdValues := c.Request.Header.Values("Forwarded"); len(fwdValues) > 0 {
		if fwdIP, valid := walkRFC7239Forwarded(fwdValues, prefixes); valid && !containsPrefixAddr(prefixes, fwdIP) {
			if mutateHeaders {
				prependForwardedClientIP(c, fwdIP.String())
			}
			return fwdIP.String()
		}
	}

	if resolvedOK {
		return resolvedIP.String()
	}
	return rawClientIP
}

func prependForwardedClientIP(c *gin.Context, clientIP string) {
	existing := strings.TrimSpace(c.GetHeader("X-Forwarded-For"))
	if existing == "" {
		c.Request.Header.Set("X-Forwarded-For", clientIP)
		return
	}
	c.Request.Header.Set("X-Forwarded-For", clientIP+", "+existing)
}

func getTrustedProxiesEnv() string {
	return os.Getenv("TRUSTED_PROXIES")
}

func currentTrustedProxyPrefixes() []netip.Prefix {
	rawEnv := strings.TrimSpace(getTrustedProxiesEnv())
	if loaded := activeTrustedProxyPrefixes.Load(); loaded != nil && loaded.rawEnv == rawEnv {
		return loaded.prefixes
	}
	entries, _, err := ResolveTrustedProxies(rawEnv)
	if err != nil {
		return defaultTrustedPrefixes
	}
	prefixes, err := parseTrustedPrefixes(entries)
	if err != nil {
		return defaultTrustedPrefixes
	}
	activeTrustedProxyPrefixes.Store(&trustedProxyCache{
		rawEnv:   rawEnv,
		prefixes: prefixes,
	})
	return prefixes
}

func mustParseTrustedPrefixes(entries []string) []netip.Prefix {
	prefixes, err := parseTrustedPrefixes(entries)
	if err != nil {
		panic(err)
	}
	return prefixes
}

func parseTrustedPrefixes(entries []string) ([]netip.Prefix, error) {
	if len(entries) == 0 {
		return []netip.Prefix{}, nil
	}
	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, raw := range entries {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, err
			}
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, err
		}
		addr = addr.Unmap().WithZone("")
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

func containsPrefixAddr(prefixes []netip.Prefix, addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	unmapped := addr.Unmap().WithZone("")
	for i := range prefixes {
		if prefixes[i].Contains(unmapped) {
			return true
		}
	}
	return false
}

func parseNormalizedAddr(raw string) (netip.Addr, bool) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"`)
	if s == "" {
		return netip.Addr{}, false
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.Unmap().WithZone(""), true
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		if addr, err := netip.ParseAddr(s[1 : len(s)-1]); err == nil {
			return addr.Unmap().WithZone(""), true
		}
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr.Unmap().WithZone(""), true
		}
	}
	return netip.Addr{}, false
}

func sanitizeForwardedList(raw string) (string, bool) {
	parts := strings.Split(raw, ",")
	changed := false
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if addr, ok := parseNormalizedAddr(trimmed); ok {
			normalized := addr.String()
			if normalized != trimmed {
				changed = true
			}
			cleaned = append(cleaned, normalized)
		} else {
			cleaned = append(cleaned, trimmed)
		}
	}
	if !changed {
		return raw, false
	}
	return strings.Join(cleaned, ", "), true
}

func walkForwardedChain(header string, trusted []netip.Prefix) (netip.Addr, bool) {
	items := strings.Split(header, ",")
	for i := len(items) - 1; i >= 0; i-- {
		addr, ok := parseNormalizedAddr(items[i])
		if !ok {
			break
		}
		if i == 0 || !containsPrefixAddr(trusted, addr) {
			return addr, true
		}
	}
	return netip.Addr{}, false
}

func walkRFC7239Forwarded(values []string, trusted []netip.Prefix) (netip.Addr, bool) {
	var hops []netip.Addr
	for _, header := range values {
		for element := range strings.SplitSeq(header, ",") {
			for param := range strings.SplitSeq(element, ";") {
				key, val, ok := strings.Cut(strings.TrimSpace(param), "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(key), "for") {
					continue
				}
				addr, valid := parseNormalizedAddr(val)
				if !valid {
					return netip.Addr{}, false
				}
				hops = append(hops, addr)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if i == 0 || !containsPrefixAddr(trusted, hops[i]) {
			return hops[i], true
		}
	}
	return netip.Addr{}, false
}
