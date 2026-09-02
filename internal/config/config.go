package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

const (
	defaultYAMLFileName = ".agbx.yaml"
	defaultYMLFileName  = ".agbx.yml"
	currentVersion      = 1
)

var ErrNotFound = errors.New("config file not found")

type Config struct {
	Image   Image `yaml:"image"`
	Version int   `yaml:"version"`
}

type Image struct {
	Name   string `yaml:"name"`
	Tag    string `yaml:"tag"`
	Digest string `yaml:"digest,omitempty"`
}

func (image Image) Reference() string {
	reference := image.Name + ":" + image.Tag
	if image.Digest == "" {
		return reference
	}

	return reference + "@" + image.Digest
}

func New() Config {
	return Config{Version: currentVersion}
}

func (configuration Config) Validate() error {
	if configuration.Version == 0 {
		return errors.New("config version is required")
	}
	if configuration.Version != currentVersion {
		return fmt.Errorf("unsupported config version %d", configuration.Version)
	}
	if strings.TrimSpace(configuration.Image.Name) == "" {
		return errors.New("config image name is required")
	}
	if strings.TrimSpace(configuration.Image.Tag) == "" {
		return errors.New("config image tag is required")
	}

	return nil
}

func Load(filePath string) (Config, error) {
	filePath = filepath.Clean(filePath)
	// #nosec G304 -- The CLI intentionally reads the configuration file selected by the user.
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", filePath, err)
	}

	var configuration Config
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", filePath, err)
	}
	if err := configuration.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config file %q: %w", filePath, err)
	}

	return configuration, nil
}

func LoadDefault(directory string) (Config, error) {
	for _, fileName := range []string{defaultYAMLFileName, defaultYMLFileName} {
		configuration, err := Load(filepath.Join(directory, fileName))
		if err == nil {
			return configuration, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return Config{}, err
		}
	}

	return Config{}, fmt.Errorf("%w in %q", ErrNotFound, directory)
}

func Create(directory string, configuration Config) error {
	if err := configuration.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	contents, err := yaml.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	filePath := filepath.Join(directory, defaultYAMLFileName)
	filePath = filepath.Clean(filePath)
	// #nosec G304 -- The destination is the canonical default configuration filename.
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create config file %q: %w", filePath, err)
	}

	if _, err := file.Write(contents); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("write config file %q: %w", filePath, errors.Join(err, closeErr))
		}

		return fmt.Errorf("write config file %q: %w", filePath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config file %q: %w", filePath, err)
	}

	return nil
}
