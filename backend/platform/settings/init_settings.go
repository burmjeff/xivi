package settings

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

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
	}

	if err := WriteSettings(appSettings); err != nil {
		return err
	}

	return nil
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
			IngestBufferMS:        1500,
			StartupTimeoutSeconds: 12,
			StallTimeoutSeconds:   10,
			IdleTimeoutSeconds:    45,
			RetryLimit:            6,
			RetryBackoffMS:        500,
			HLSSegmentSeconds:     2,
			HLSPlaylistLength:     10,
			ClientBufferMB:        2,
			TLSVerify:             true,
			UserAgent:             "Xivi 1.0",
		},
		Vector: Vector{
			BatchSize:       100,
			ParallelBatches: 10,
			Timeout:         90,
			CacheSize:       500,
		},
		VirtualTuner: VirtualTuner{
			TunerCount: 6,
		},
	}

	return &defaults, nil
}

func WriteSettings(settings *AppSettings) error {
	config := fmt.Sprintf("%s/config.yaml", CONFIG_PATH)
	yamlData, err := yaml.Marshal(&settings)
	if err != nil {
		return fmt.Errorf("error while marshaling. %v", err)
	}
	err = os.WriteFile(config, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("unable to write data into the settings file: %v", err)
	}

	// Update log level if it changed
	if settings.Application.LogLevel != APP_SETTINGS.Application.LogLevel {
		zerolog.SetGlobalLevel(zerolog.Level(settings.Application.LogLevel))
		log.Debug().Msgf("Log level changed to: %d", settings.Application.LogLevel)
	}

	APP_SETTINGS = settings
	return nil
}

func InitPaths() error {
	if _, err := os.Stat(CONFIG_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(CONFIG_PATH, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(SERVE_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(SERVE_PATH, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(M3U_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(M3U_FILEPATH, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(EPG_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(EPG_FILEPATH, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(LOGO_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(LOGO_FILEPATH, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(STREAM_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(STREAM_FILEPATH, os.ModePerm)
		if err != nil {
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

	destination, err := os.Create(fmt.Sprintf("%s/xivi_channel.png", LOGO_FILEPATH)) //create the destination file
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
