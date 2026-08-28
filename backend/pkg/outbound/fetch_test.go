package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestBlockedAddressClasses(t *testing.T) {
	blocked := []string{"127.0.0.1", "0.0.0.0", "100.100.100.200", "169.254.169.254", "198.18.0.1", "224.0.0.1", "::1", "fe80::1", "fc00::1", "fd00:ec2::254", "10.0.0.1"}
	for _, raw := range blocked {
		if allowedIP(net.ParseIP(raw), false) {
			t.Errorf("%s should be blocked", raw)
		}
	}
	if !allowedIP(net.ParseIP("192.168.1.20"), true) {
		t.Fatal("explicit LAN policy should allow RFC1918 addresses")
	}
	if allowedIP(net.ParseIP("127.0.0.1"), true) || allowedIP(net.ParseIP("169.254.169.254"), true) {
		t.Fatal("LAN policy must never permit loopback or link-local metadata")
	}
}

func TestValidateRejectsLoopback(t *testing.T) {
	_, err := Validate(context.Background(), "http://127.0.0.1/private", true)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want blocked address", err)
	}
}

func withResolver(t *testing.T, resolver func(context.Context, string) ([]net.IPAddr, error)) {
	t.Helper()
	previous := lookupIPAddr
	lookupIPAddr = resolver
	t.Cleanup(func() { lookupIPAddr = previous })
}

func TestValidateRejectsMixedPublicAndPrivateAnswers(t *testing.T) {
	withResolver(t, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.8")}}, nil
	})
	_, err := Validate(context.Background(), "https://feed.example/channels.m3u", false)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want blocked mixed DNS answer", err)
	}
}

func TestValidateResolvesEveryRequestToResistRebinding(t *testing.T) {
	lookups := 0
	withResolver(t, func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		if lookups == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	if _, err := Validate(context.Background(), "https://feed.example/channels.m3u", false); err != nil {
		t.Fatalf("first validation failed: %v", err)
	}
	if _, err := Validate(context.Background(), "https://feed.example/channels.m3u", false); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("second validation got %v, want blocked rebound answer", err)
	}
}

func TestRedirectValidationRejectsPrivateDestination(t *testing.T) {
	withResolver(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "redirect.example" {
			return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	})
	client := NewClient(Policy{})
	request, err := http.NewRequest(http.MethodGet, "http://redirect.example/latest/meta-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(request, nil); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want blocked redirect destination", err)
	}
}

func TestAlternativeLoopbackEncodingsFailClosed(t *testing.T) {
	withResolver(t, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	for _, raw := range []string{
		"http://2130706433/private",
		"http://0x7f000001/private",
		"http://0177.0.0.1/private",
		"http://[::ffff:127.0.0.1]/private",
	} {
		if _, err := Validate(context.Background(), raw, true); err == nil {
			t.Errorf("%s should have been rejected", raw)
		}
	}
}
