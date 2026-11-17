package system_setting

import (
	"net"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	NetworkPrimaryDNSOptionKey  = "network_setting.primary_dns"
	NetworkFallbackDNSOptionKey = "network_setting.fallback_dns"
)

// NetworkSetting controls outbound DNS resolution behavior.
type NetworkSetting struct {
	PrimaryDNS  string `json:"primary_dns"`
	FallbackDNS string `json:"fallback_dns"`
}

var networkSetting = NetworkSetting{}
var networkRuntime atomic.Pointer[NetworkSetting]

func init() {
	config.GlobalConfig.Register("network_setting", &networkSetting)
	initial := networkSetting
	networkRuntime.Store(&initial)
}

func GetNetworkSetting() NetworkSetting {
	if current := networkRuntime.Load(); current != nil {
		return *current
	}
	return NetworkSetting{}
}

func SetNetworkSetting(primaryDNS, fallbackDNS string) {
	next := NetworkSetting{
		PrimaryDNS:  strings.TrimSpace(primaryDNS),
		FallbackDNS: strings.TrimSpace(fallbackDNS),
	}
	networkRuntime.Store(&next)
}

// NormalizeDNSServerEndpoint validates a DNS server value (IP or IP:port) and
// returns its canonical host:port representation (defaulting to port 53).
// Empty input returns ("", nil).
func NormalizeDNSServerEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	host := trimmed
	port := "53"
	if splitHost, splitPort, err := net.SplitHostPort(trimmed); err == nil {
		host = strings.TrimSpace(splitHost)
		port = strings.TrimSpace(splitPort)
	} else if inner, ok := strings.CutPrefix(trimmed, "["); ok {
		if inner, ok = strings.CutSuffix(inner, "]"); ok {
			host = strings.TrimSpace(inner)
		}
	}

	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return "", common.NewMessage("DNS server must be a valid IPv4 or IPv6 address (with optional :port)")
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return "", common.NewMessage("DNS server port must be between 1 and 65535")
	}

	return net.JoinHostPort(ip.String(), strconv.Itoa(portNum)), nil
}

// ValidateDNSServerOption validates the option value for primary/fallback DNS settings.
func ValidateDNSServerOption(value string) error {
	_, err := NormalizeDNSServerEndpoint(value)
	return err
}

// ConfiguredDNSServers returns the deduplicated canonical host:port endpoints
// for primary and fallback DNS servers in priority order.
func ConfiguredDNSServers() []string {
	cfg := GetNetworkSetting()
	servers := make([]string, 0, 2)
	if primary, err := NormalizeDNSServerEndpoint(cfg.PrimaryDNS); err == nil && primary != "" {
		servers = append(servers, primary)
	}
	if fallback, err := NormalizeDNSServerEndpoint(cfg.FallbackDNS); err == nil && fallback != "" {
		if len(servers) == 0 || servers[0] != fallback {
			servers = append(servers, fallback)
		}
	}
	return servers
}
