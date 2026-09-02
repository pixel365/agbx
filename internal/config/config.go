package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

const (
	defaultYAMLFileName = ".agbx.yaml"
	defaultYMLFileName  = ".agbx.yml"
	currentVersion      = 1
	defaultContent      = "version: 1\n"
)

var ErrNotFound = errors.New("config file not found")

type Config struct {
	Version int `yaml:"version"`
}

func (с Config) Validate() error {
	if с.Version == 0 {
		return errors.New("config version is required")
	}
	if с.Version != currentVersion {
		return fmt.Errorf("unsupported config version %d", с.Version)
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

func CreateDefault(directory string) error {
	filePath := filepath.Join(directory, defaultYAMLFileName)
	filePath = filepath.Clean(filePath)
	// #nosec G304 -- The destination is the canonical default configuration filename.
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create config file %q: %w", filePath, err)
	}

	if _, err := file.WriteString(defaultContent); err != nil {
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
