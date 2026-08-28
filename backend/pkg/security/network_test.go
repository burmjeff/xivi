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
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if observed.Scheme != "http" || observed.TrustedProxy {
		t.Fatalf("untrusted forwarded headers changed transport: %#v", observed)
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
