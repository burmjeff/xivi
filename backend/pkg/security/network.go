package security

import (
	"net"
	"net/url"
	"strings"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

type RequestNetwork struct {
	IP               net.IP
	Scheme           string
	TrustedProxy     bool
	DirectTrustedLAN bool
}

func RequestNetworkInfo(c *fiber.Ctx) RequestNetwork {
	policy := settings.Current().Security
	return requestNetworkInfo(c, policy)
}

func requestNetworkInfo(c *fiber.Ctx, policy settings.Security) RequestNetwork {
	direct := c.Context().RemoteIP()
	trustedProxy := ipInCIDRs(direct, policy.TrustedProxyCIDRs)
	clientIP := direct
	// Derive the direct transport from the socket, never from request headers.
	// Forwarded scheme and client IP are considered only after the direct peer
	// has matched an explicit reverse-proxy CIDR.
	scheme := "http"
	if c.Context().IsTLS() {
		scheme = "https"
	}
	if trustedProxy {
		if forwarded := lastForwardedValue(c.Get("X-Forwarded-For")); forwarded != "" {
			if parsed := net.ParseIP(strings.TrimSpace(forwarded)); parsed != nil {
				clientIP = parsed
			}
		}
		if forwardedProto := strings.ToLower(lastForwardedValue(c.Get("X-Forwarded-Proto"))); forwardedProto == "http" || forwardedProto == "https" {
			scheme = forwardedProto
		}
	}
	trustedLAN := !trustedProxy && isTrustedLAN(clientIP, policy.TrustedLANCIDRs)
	return RequestNetwork{IP: clientIP, Scheme: scheme, TrustedProxy: trustedProxy, DirectTrustedLAN: trustedLAN}
}

// The configured edge proxy appends or overwrites the value it observed. The
// right-most hop is therefore authoritative; taking the first value would let
// a client-supplied prefix influence transport scope or rate-limit identity.
func lastForwardedValue(value string) string {
	parts := strings.Split(value, ",")
	for index := len(parts) - 1; index >= 0; index-- {
		if candidate := strings.TrimSpace(parts[index]); candidate != "" {
			return candidate
		}
	}
	return ""
}

func IsTrustedLAN(ip net.IP) bool {
	return isTrustedLAN(ip, settings.Current().Security.TrustedLANCIDRs)
}

func isTrustedLAN(ip net.IP, trustedCIDRs []string) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	return ipInCIDRs(ip, trustedCIDRs)
}

func ipInCIDRs(ip net.IP, cidrs []string) bool {
	if ip == nil {
		return false
	}
	for _, raw := range cidrs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func TransportScope(c *fiber.Ctx) (string, bool) {
	policy := settings.Current().Security
	network := requestNetworkInfo(c, policy)
	if network.Scheme == "https" {
		return "https", true
	}
	if policy.AllowLANHTTP && network.DirectTrustedLAN {
		return "lan_http", true
	}
	return "", false
}

func BaseURLForRequest(c *fiber.Ctx) string {
	policy := settings.Current().Security
	network := requestNetworkInfo(c, policy)
	if network.Scheme == "https" && policy.PublicBaseURL != "" {
		return policy.PublicBaseURL
	}
	return policy.LocalBaseURL
}

func ValidBaseURL(value string, requireHTTPS bool) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if requireHTTPS {
		return parsed.Scheme == "https"
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
