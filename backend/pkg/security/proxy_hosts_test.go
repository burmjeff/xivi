package security

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
	"xivi/backend/platform/settings"
)

func TestTrustedProxyHostnameUsesPrivateDNSAddress(t *testing.T) {
	lookups := 0
	resolver := newProxyHostnameResolver(time.Minute, func(_ context.Context, host string) ([]net.IPAddr, error) {
		lookups++
		if host != "reverse-proxy" {
			t.Fatalf("unexpected hostname %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("172.20.0.8")}}, nil
	})

	if !resolver.contains(net.ParseIP("172.20.0.8"), []string{"reverse-proxy"}) {
		t.Fatal("resolved Docker proxy address was not trusted")
	}
	if !resolver.contains(net.ParseIP("172.20.0.8"), []string{"reverse-proxy"}) || lookups != 1 {
		t.Fatalf("fresh result was not cached; lookups=%d", lookups)
	}
	if resolver.contains(net.ParseIP("172.20.0.9"), []string{"reverse-proxy"}) || lookups != 1 {
		t.Fatalf("an unlisted peer bypassed or refreshed the fresh cache; lookups=%d", lookups)
	}
}

func TestTrustedProxyHostnameRefreshReplacesContainerAddress(t *testing.T) {
	current := "172.20.0.8"
	resolver := newProxyHostnameResolver(time.Minute, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP(current)}}, nil
	})
	configured := []string{"reverse-proxy"}
	if !resolver.contains(net.ParseIP(current), configured) {
		t.Fatal("initial address was not trusted")
	}

	current = "172.20.0.9"
	resolver.mu.Lock()
	resolver.expires = time.Now().Add(-time.Second)
	resolver.mu.Unlock()
	if !resolver.contains(net.ParseIP(current), configured) {
		t.Fatal("replacement container address was not trusted after refresh")
	}
	if resolver.contains(net.ParseIP("172.20.0.8"), configured) {
		t.Fatal("stale container address remained trusted after refresh")
	}
}

func TestTrustedProxyHostnameLookupFailureFailsClosed(t *testing.T) {
	fail := false
	resolver := newProxyHostnameResolver(time.Minute, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		if fail {
			return nil, errors.New("resolver unavailable")
		}
		return []net.IPAddr{{IP: net.ParseIP("172.20.0.8")}}, nil
	})
	configured := []string{"reverse-proxy"}
	if !resolver.contains(net.ParseIP("172.20.0.8"), configured) {
		t.Fatal("initial address was not trusted")
	}

	fail = true
	resolver.mu.Lock()
	resolver.expires = time.Now().Add(-time.Second)
	resolver.mu.Unlock()
	if resolver.contains(net.ParseIP("172.20.0.8"), configured) {
		t.Fatal("stale address remained trusted after a failed refresh")
	}
}

func TestTrustedProxyHostnameRejectsPublicResolution(t *testing.T) {
	resolver := newProxyHostnameResolver(time.Minute, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.8")}}, nil
	})
	if resolver.contains(net.ParseIP("203.0.113.8"), []string{"proxy.example.test"}) {
		t.Fatal("public DNS result was trusted instead of requiring an explicit CIDR")
	}
}

func TestTrustedProxyPolicyAcceptsCIDROrHostname(t *testing.T) {
	original := trustedProxyHostnameResolver
	trustedProxyHostnameResolver = newProxyHostnameResolver(time.Minute, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("172.20.0.8")}}, nil
	})
	t.Cleanup(func() { trustedProxyHostnameResolver = original })

	if !trustedProxyForPolicy(net.ParseIP("10.0.0.2"), settings.Security{TrustedProxyCIDRs: []string{"10.0.0.0/24"}}) {
		t.Fatal("trusted proxy CIDR stopped working")
	}
	if !trustedProxyForPolicy(net.ParseIP("172.20.0.8"), settings.Security{TrustedProxyHosts: []string{"reverse-proxy"}}) {
		t.Fatal("trusted proxy hostname was not accepted")
	}
}
