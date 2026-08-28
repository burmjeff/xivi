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
	trustedProxy := trustedProxyForPolicy(direct, policy)
	forwardedRequest := hasForwardingHeaders(c)
	clientIP := direct
	// Derive the direct transport from the socket, never from request headers.
	// Forwarded scheme and client IP are considered only after the direct peer
	// has matched an explicit reverse-proxy CIDR or resolved private hostname.
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
	// A reverse proxy commonly connects from an RFC1918 Docker or LAN address.
	// Without this check an unconfigured public proxy could inherit the automatic
	// direct-LAN exception merely because its socket peer is private. Direct LAN
	// clients remain automatic; proxy-shaped requests require an explicit trusted
	// proxy identity before either forwarded metadata or HTTPS scope is accepted.
	trustedLAN := !trustedProxy && !forwardedRequest && trustedLANForPolicy(clientIP, policy)
	return RequestNetwork{IP: clientIP, Scheme: scheme, TrustedProxy: trustedProxy, DirectTrustedLAN: trustedLAN}
}

func trustedProxyForPolicy(ip net.IP, policy settings.Security) bool {
	return ipInCIDRs(ip, policy.TrustedProxyCIDRs) ||
		trustedProxyHostnameResolver.contains(ip, policy.TrustedProxyHosts)
}

func hasForwardingHeaders(c *fiber.Ctx) bool {
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if strings.TrimSpace(c.Get(name)) != "" {
			return true
		}
	}
	return false
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
	return trustedLANForPolicy(ip, settings.Current().Security)
}

// trustedLANForPolicy uses an explicit allowlist whenever one is configured.
// Without one, directly connected loopback, RFC1918, and IPv6 ULA clients are
// treated as local. Reverse-proxy classification happens first, so a trusted
// proxy peer is never reclassified as a LAN client.
func trustedLANForPolicy(ip net.IP, policy settings.Security) bool {
	if len(policy.TrustedLANCIDRs) > 0 {
		return isTrustedLAN(ip, policy.TrustedLANCIDRs)
	}
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
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

// MediaBaseURLForRequest optionally separates compatibility-player traffic
// from the browser/WAF hostname. Browser playback continues to use same-origin
// paths, while generated device outputs use this key-authenticated media host.
func MediaBaseURLForRequest(c *fiber.Ctx) string {
	policy := settings.Current().Security
	network := requestNetworkInfo(c, policy)
	if network.Scheme == "https" {
		if policy.PublicMediaBaseURL != "" {
			return policy.PublicMediaBaseURL
		}
		if policy.PublicBaseURL != "" {
			return policy.PublicBaseURL
		}
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
