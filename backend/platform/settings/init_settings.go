package settings

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

func InitSettings() error {
	appSettings, err := SetDefaults()
	if err != nil {
		return err
	}
	config := fmt.Sprintf("%s/config.yaml", CONFIG_PATH)

	if err := ValidateConfigPath(config); err != nil {
		// Defaults are already populated. WriteSettings below creates the file.
	} else {
		file, err := os.Open(config)
		if err != nil {
			return err
		}
		defer file.Close()

		// Init new YAML decode
		d := yaml.NewDecoder(file)

		// Start YAML decoding from file
		if err := d.Decode(appSettings); err != nil {
			return err
		}

	}

	if IsRunningInDockerContainer() {
		if env, exists := os.LookupEnv("APP_NAME"); exists {
			appSettings.Application.AppName = env
		}
		if env, exists := os.LookupEnv("APP_VERSION"); exists {
			appSettings.Application.AppVersion = env
		}
		if env, exists := os.LookupEnv("TZ"); exists {
			appSettings.Application.TZ = env
		}
		if env, exists := os.LookupEnv("SERVER_HOST"); exists {
			appSettings.Server.Host = env
		}
		if env, exists := os.LookupEnv("SERVER_PORT"); exists {
			serverPort, err := strconv.Atoi(env)
			if err != nil {
				log.Error().Msg(fmt.Sprintf("Not a valid Port: %s", env))
			} else {
				appSettings.Server.Port = serverPort
			}
		}
		if env, exists := os.LookupEnv("SERVER_READ_TIMEOUT"); exists {
			timeout, err := strconv.Atoi(env)
			if err != nil {
				log.Error().Msg(fmt.Sprintf("Not a valid Server Read Timeout: %s", env))
			} else {
				appSettings.Server.ReadTimeout = timeout
			}
		}
		if env, exists := os.LookupEnv("LOG_LEVEL"); exists {
			level, err := strconv.Atoi(env)
			if err != nil {
				log.Error().Msg(fmt.Sprintf("Not a log level: %s", env))
			} else {
				appSettings.Application.LogLevel = level
				log.Debug().Msgf("Log level set to: %d", level)
			}
		}
		if env, exists := os.LookupEnv("PUBLIC_BASE_URL"); exists {
			appSettings.Security.PublicBaseURL = strings.TrimRight(strings.TrimSpace(env), "/")
		}
		if env, exists := os.LookupEnv("LOCAL_BASE_URL"); exists {
			appSettings.Security.LocalBaseURL = strings.TrimRight(strings.TrimSpace(env), "/")
		}
		if env, exists := os.LookupEnv("TRUSTED_PROXY_CIDRS"); exists {
			appSettings.Security.TrustedProxyCIDRs = splitCIDRs(env)
		}
		if env, exists := os.LookupEnv("TRUSTED_LAN_CIDRS"); exists {
			appSettings.Security.TrustedLANCIDRs = splitCIDRs(env)
		}
		if env, exists := os.LookupEnv("ALLOW_LAN_HTTP"); exists {
			appSettings.Security.AllowLANHTTP, _ = strconv.ParseBool(env)
		}
	}

	if appSettings.Streaming.HedgeTimeoutSeconds == 0 {
		appSettings.Streaming.HedgeTimeoutSeconds = 3
	}
	// The serve root is fixed by the process working directory and mounted
	// volume. A historical config value must not imply that it can move live.
	appSettings.Application.ServePath = SERVE_PATH
	if err := WriteSettings(appSettings); err != nil {
		return err
	}

	return nil
}

func splitCIDRs(value string) []string {
	result := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func SetDefaults() (*AppSettings, error) {
	defaults := AppSettings{
		Application: Application{
			AppName:    "Xivi",
			AppVersion: "1.0",
			TZ:         "America/New_York",
			ServePath:  "./serve",
			LogLevel:   3,
			UpdateCron: "0 0 * * *",
		},
		Server: Server{
			Host:        "127.0.0.1",
			Port:        3000,
			ReadTimeout: 60,
		},
		Playlist: Playlist{
			Tvgid_match: true,
			Name_match:  true,
			Name_score:  0.96,
		},
		Streaming: Streaming{
			Proxy:                 true,
			IngestBufferMS:        750,
			StartupTimeoutSeconds: 12,
			StartupHedgeMS:        750,
			StallTimeoutSeconds:   5,
			HedgeTimeoutSeconds:   3,
			IdleTimeoutSeconds:    45,
			RetryLimit:            6,
			RetryBackoffMS:        500,
			HLSSegmentSeconds:     1,
			HLSPlaylistLength:     8,
			HLSCompatibilityMode:  true,
			ClientBufferMB:        2,
			PrewarmChannels:       2,
			TLSVerify:             true,
			UserAgent:             "Xivi 1.0",
		},
		Vector: Vector{
			BatchSize:       100,
			ParallelBatches: 10,
			Timeout:         90,
			CacheSize:       500,
		},
		Maintenance: Maintenance{
			CleanupIntervalHours:      6,
			OperationJobRetentionDays: 30,
			StreamDiagnosticsDays:     14,
			MaximumOperationJobs:      2000,
			MaximumStreamSessions:     2000,
		},
		VirtualTuner: VirtualTuner{
			TunerCount: 6,
		},
		Security: Security{
			LocalBaseURL:       "http://127.0.0.1:3000",
			AllowLANHTTP:       true,
			AuditRetentionDays: 90,
			MaximumAuditEvents: 100000,
		},
	}

	return &defaults, nil
}

func WriteSettings(next *AppSettings) error {
	normalizeMaintenance(&next.Maintenance)
	normalizeSecurity(&next.Security)
	if err := ValidateSecuritySettings(next.Security); err != nil {
		return err
	}
	config := fmt.Sprintf("%s/config.yaml", CONFIG_PATH)
	yamlData, err := yaml.Marshal(&next)
	if err != nil {
		return fmt.Errorf("error while marshaling. %v", err)
	}
	if err := writeConfigAtomically(config, yamlData); err != nil {
		return err
	}

	// Update log level if it changed
	previous := Current()
	if next.Application.LogLevel != previous.Application.LogLevel {
		zerolog.SetGlobalLevel(zerolog.Level(next.Application.LogLevel))
		log.Debug().Msgf("Log level changed to: %d", next.Application.LogLevel)
	}

	publish(next)
	return nil
}

func writeConfigAtomically(config string, data []byte) error {
	directory := filepath.Dir(config)
	temporary, err := os.CreateTemp(directory, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("unable to create temporary settings file: %v", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("unable to secure temporary settings file: %v", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("unable to write temporary settings file: %v", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("unable to sync temporary settings file: %v", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("unable to close temporary settings file: %v", err)
	}
	if err := os.Rename(temporaryName, config); err != nil {
		return fmt.Errorf("unable to replace the settings file: %v", err)
	}
	if err := os.Chmod(config, 0600); err != nil {
		return fmt.Errorf("unable to secure the settings file: %v", err)
	}
	return nil
}

// PreserveDeploymentSettings prevents a settings API request from changing
// values that are captured by the process, container, reverse proxy, or volume
// layout. These values remain configurable through deployment configuration.
func PreserveDeploymentSettings(next *AppSettings) {
	current := Current()
	next.Application.ServePath = current.Application.ServePath
	next.Server = current.Server
}

func ValidateSecuritySettings(security Security) error {
	validateBase := func(raw string, requireHTTPS bool) error {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return fmt.Errorf("base URL must contain only a scheme and host")
		}
		if requireHTTPS && parsed.Scheme != "https" {
			return fmt.Errorf("public base URL must use HTTPS")
		}
		if !requireHTTPS && parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("local base URL must use HTTP or HTTPS")
		}
		return nil
	}
	production := strings.EqualFold(strings.TrimSpace(os.Getenv("XIVI_PRODUCTION")), "true")
	if security.PublicBaseURL != "" {
		if err := validateBase(security.PublicBaseURL, true); err != nil {
			return err
		}
	} else if production {
		return fmt.Errorf("PUBLIC_BASE_URL is required in production")
	}
	if err := validateBase(security.LocalBaseURL, false); err != nil {
		return err
	}
	for _, item := range append(append([]string{}, security.TrustedProxyCIDRs...), security.TrustedLANCIDRs...) {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(item)); err != nil {
			return fmt.Errorf("invalid trusted network CIDR")
		}
	}
	if production && len(security.TrustedProxyCIDRs) == 0 {
		return fmt.Errorf("TRUSTED_PROXY_CIDRS is required in production")
	}
	if security.AllowLANHTTP && production && len(security.TrustedLANCIDRs) == 0 {
		return fmt.Errorf("TRUSTED_LAN_CIDRS is required when LAN HTTP is enabled in production")
	}
	return nil
}

func normalizeSecurity(security *Security) {
	security.PublicBaseURL = strings.TrimRight(strings.TrimSpace(security.PublicBaseURL), "/")
	security.LocalBaseURL = strings.TrimRight(strings.TrimSpace(security.LocalBaseURL), "/")
	if security.LocalBaseURL == "" {
		security.LocalBaseURL = "http://127.0.0.1:3000"
	}
	if security.AuditRetentionDays < 1 || security.AuditRetentionDays > 3650 {
		security.AuditRetentionDays = 90
	}
	if security.MaximumAuditEvents < 1000 || security.MaximumAuditEvents > 1000000 {
		security.MaximumAuditEvents = 100000
	}
}

func normalizeMaintenance(maintenance *Maintenance) {
	if maintenance.CleanupIntervalHours < 1 || maintenance.CleanupIntervalHours > 168 {
		maintenance.CleanupIntervalHours = 6
	}
	if maintenance.OperationJobRetentionDays < 1 || maintenance.OperationJobRetentionDays > 365 {
		maintenance.OperationJobRetentionDays = 30
	}
	if maintenance.StreamDiagnosticsDays < 1 || maintenance.StreamDiagnosticsDays > 365 {
		maintenance.StreamDiagnosticsDays = 14
	}
	if maintenance.MaximumOperationJobs < 100 || maintenance.MaximumOperationJobs > 100000 {
		maintenance.MaximumOperationJobs = 2000
	}
	if maintenance.MaximumStreamSessions < 100 || maintenance.MaximumStreamSessions > 100000 {
		maintenance.MaximumStreamSessions = 2000
	}
}

func InitPaths() error {
	for _, path := range []string{CONFIG_PATH, SERVE_PATH, M3U_FILEPATH, EPG_FILEPATH, LOGO_FILEPATH, STREAM_FILEPATH} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

func CopyDefaultLogo() {
	var source *os.File
	var err error
	for _, candidate := range []string{"/xivi/xivi_channel.png", "xivi_channel.png"} {
		source, err = os.Open(candidate)
		if err == nil {
			break
		}
	}
	if source == nil {
		log.Error().Err(err).Msg("default Signal Tile asset could not be opened")
		return
	}
	defer source.Close()

	destination, err := os.OpenFile(fmt.Sprintf("%s/xivi_channel.png", LOGO_FILEPATH), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Err(err)
		return
	}
	defer destination.Close()
	_, err = io.Copy(destination, source) //copy the contents of source to destination file
	if err != nil {
		log.Err(err)
		return
	}
}

func ValidateConfigPath(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return fmt.Errorf("'%s' is a directory, not config file", path)
	}
	return nil
}

func IsRunningInDockerContainer() bool {
	// docker creates a .dockerenv file at the root
	// of the directory tree inside the container.
	// if this file exists then the viewer is running
	// from inside a container so return true

	if _, err := os.Stat("/.dockerenv"); err == nil {

		return true
	}

	return false
}
