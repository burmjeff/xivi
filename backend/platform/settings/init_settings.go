package settings

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

func InitSettings() error {
	appSettings := &AppSettings{}
	config := fmt.Sprintf("%s/config.yaml", CONFIG_PATH)

	if err := ValidateConfigPath(config); err != nil {
		if appSettings, err = SetDefaults(); err != nil {
			return err
		}
		if err := WriteSettings(appSettings); err != nil {
			return err
		}
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
		if env, exists := os.LookupEnv("MODEL_NAME"); exists {
			appSettings.Application.Model = env
		}
		if env, exists := os.LookupEnv("LOG_LEVEL"); exists {
			level, err := strconv.Atoi(env)
			if err != nil {
				log.Error().Msg(fmt.Sprintf("Not a log level: %s", env))
			} else {
				appSettings.Application.LogLevel = level
				zerolog.SetGlobalLevel(zerolog.Level(level))
			}
		}
		if err := WriteSettings(appSettings); err != nil {
			log.Fatal().Msg(err.Error())
		}
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
			Model:      "sentence-transformers/all-MiniLM-L6-v2",
			LogLevel:   3,
			UpdateCron: "0 0 * * *",
		},
		Server: Server{
			Host:        "0.0.0.0",
			Port:        3000,
			ReadTimeout: 60,
		},
		Streaming: Streaming{
			Proxy:     true,
			Buffer:    0,
			UserAgent: "Xivi 1.0",
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

	APP_SETTINGS = settings
	return nil
}

func InitPaths() error {

	if IsRunningInDockerContainer() {
		CONFIG_PATH = "/config"
		SERVE_PATH = "/serve"
		M3U_FILEPATH = fmt.Sprintf("%s/m3u", SERVE_PATH)
		EPG_FILEPATH = fmt.Sprintf("%s/epg", SERVE_PATH)
		LOGO_FILEPATH = fmt.Sprintf("%s/logo", SERVE_PATH)
		STREAM_FILEPATH = fmt.Sprintf("%s/stream", SERVE_PATH)
		MODEL_PATH = fmt.Sprintf("%s/models", CONFIG_PATH)
	}
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
	if _, err := os.Stat(MODEL_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(MODEL_PATH, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
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
