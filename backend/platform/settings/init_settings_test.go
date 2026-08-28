package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreserveDeploymentSettings(t *testing.T) {
	current := Current()
	next := *current
	next.Application.ServePath = "/untrusted/path"
	next.Server = Server{Host: "example.test", Port: 9999, ReadTimeout: 1}

	PreserveDeploymentSettings(&next)

	if next.Application.ServePath != current.Application.ServePath {
		t.Fatalf("serve path changed through runtime settings: %q", next.Application.ServePath)
	}
	if next.Server != current.Server {
		t.Fatalf("server deployment settings changed through runtime settings: %#v", next.Server)
	}
}

func TestWriteSettingsAtomicallyReplacesExistingConfig(t *testing.T) {
	originalPath := CONFIG_PATH
	originalSettings := Current()
	CONFIG_PATH = t.TempDir()
	t.Cleanup(func() {
		CONFIG_PATH = originalPath
		publish(originalSettings)
	})

	first, err := SetDefaults()
	if err != nil {
		t.Fatal(err)
	}
	first.Application.AppName = "First"
	if err := WriteSettings(first); err != nil {
		t.Fatalf("write first settings: %v", err)
	}

	second := *first
	second.Application.AppName = "Second"
	if err := WriteSettings(&second); err != nil {
		t.Fatalf("replace settings: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(CONFIG_PATH, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || Current().Application.AppName != "Second" {
		t.Fatal("replacement settings were not published")
	}
	matches, err := filepath.Glob(filepath.Join(CONFIG_PATH, ".config-*.yaml.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary settings files were not cleaned up: %v", matches)
	}
}

func TestProductionSecurityAllowsLocalOnlyDeployment(t *testing.T) {
	t.Setenv("XIVI_PRODUCTION", "true")
	security := Security{
		LocalBaseURL: "http://192.168.1.10:3000",
		AllowLANHTTP: true,
	}
	if err := ValidateSecuritySettings(security); err != nil {
		t.Fatalf("local-only production deployment was rejected: %v", err)
	}
}

func TestProductionPublicDeploymentRequiresTrustedProxy(t *testing.T) {
	t.Setenv("XIVI_PRODUCTION", "true")
	security := Security{
		PublicBaseURL: "https://tv.example.com",
		LocalBaseURL:  "http://127.0.0.1:3000",
	}
	err := ValidateSecuritySettings(security)
	if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXY_CIDRS") {
		t.Fatalf("public production deployment did not require a trusted proxy: %v", err)
	}
}
