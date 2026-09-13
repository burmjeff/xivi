package security

import (
	"net"
	"net/http/httptest"
	"testing"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

func TestForwardedHeadersRequireTrustedDirectPeer(t *testing.T) {
	original := settings.Current().Security
	t.Cleanup(func() { settings.Current().Security = original })
	settings.Current().Security.TrustedProxyCIDRs = nil

	var observed RequestNetwork
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		observed = RequestNetworkInfo(c)
		return c.SendStatus(fiber.StatusNoContent)
	})
	request := httptest.NewRequest(fiber.MethodGet, "http://xivi.test/", nil)
	// Use origin-form so Fiber's DumpRequest includes the required Host header.
	request.RequestURI = request.URL.RequestURI()
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if observed.Scheme != "http" || observed.TrustedProxy || observed.DirectTrustedLAN {
		t.Fatalf("untrusted forwarded headers changed transport: %#v", observed)
	}
}

func TestUnconfiguredPrivateProxyCannotInheritAutomaticLANTrust(t *testing.T) {
	original := settings.Current().Security
	t.Cleanup(func() { settings.Current().Security = original })
	settings.Current().Security.TrustedProxyCIDRs = nil
	settings.Current().Security.TrustedLANCIDRs = nil
	settings.Current().Security.AllowLANHTTP = true

	var observed RequestNetwork
	var allowed bool
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		observed = RequestNetworkInfo(c)
		_, allowed = TransportScope(c)
		return c.SendStatus(fiber.StatusNoContent)
	})
	request := httptest.NewRequest(fiber.MethodGet, "http://xivi.test/", nil)
	request.RequestURI = request.URL.RequestURI()
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "203.0.113.25")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if observed.TrustedProxy || observed.DirectTrustedLAN || allowed {
		t.Fatalf("unconfigured proxy inherited LAN transport: network=%#v allowed=%v", observed, allowed)
	}
}

func TestTrustedProxyUsesItsRightMostForwardedValues(t *testing.T) {
	original := settings.Current().Security
	t.Cleanup(func() { settings.Current().Security = original })
	settings.Current().Security.TrustedProxyCIDRs = []string{"0.0.0.0/0", "::/0"}

	var observed RequestNetwork
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		observed = RequestNetworkInfo(c)
		return c.SendStatus(fiber.StatusNoContent)
	})
	request := httptest.NewRequest(fiber.MethodGet, "http://xivi.test/", nil)
	request.RequestURI = request.URL.RequestURI()
	request.Header.Set("X-Forwarded-Proto", "https, http")
	request.Header.Set("X-Forwarded-For", "10.0.0.8, 198.51.100.20")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if observed.Scheme != "http" || !observed.IP.Equal(net.ParseIP("198.51.100.20")) {
		t.Fatalf("client-controlled prefix influenced proxy metadata: %#v", observed)
	}
}

func TestPublicHTTPSProxyIsAnAllowedSessionTransport(t *testing.T) {
	original := settings.Current().Security
	t.Cleanup(func() { settings.Current().Security = original })
	settings.Current().Security.TrustedProxyCIDRs = []string{"0.0.0.0/0", "::/0"}

	var scope string
	var allowed bool
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		scope, allowed = TransportScope(c)
		return c.SendStatus(fiber.StatusNoContent)
	})
	request := httptest.NewRequest(fiber.MethodGet, "http://xivi.test/", nil)
	request.RequestURI = request.URL.RequestURI()
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !allowed || scope != "https" {
		t.Fatalf("public HTTPS proxy transport returned scope=%q allowed=%v", scope, allowed)
	}
}

func TestEmptyLANAllowlistTrustsOnlyLocalAddresses(t *testing.T) {
	policy := settings.Security{}
	for _, address := range []string{"127.0.0.1", "192.168.1.25", "10.20.30.40", "fd00::25"} {
		if !trustedLANForPolicy(net.ParseIP(address), policy) {
			t.Fatalf("empty LAN allowlist did not trust local client %s", address)
		}
	}
	if trustedLANForPolicy(net.ParseIP("203.0.113.25"), policy) {
		t.Fatal("empty LAN allowlist trusted a public client address")
	}
}

func TestExplicitLANAllowlistReplacesAutomaticPrivateTrust(t *testing.T) {
	policy := settings.Security{TrustedLANCIDRs: []string{"10.20.0.0/16"}}
	if !trustedLANForPolicy(net.ParseIP("10.20.30.40"), policy) {
		t.Fatal("explicit LAN client was rejected")
	}
	if trustedLANForPolicy(net.ParseIP("192.168.1.25"), policy) {
		t.Fatal("unlisted private client retained automatic trust")
	}
}
