package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	defaultYAMLFileName = ".agbx.yaml"
	defaultYMLFileName  = ".agbx.yml"
)

var ErrNotFound = errors.New("config file not found")

type Config struct{}

func Load(filePath string) (Config, error) {
	filePath = filepath.Clean(filePath)
	// #nosec G304 -- The CLI intentionally reads the configuration file selected by the user.
	if _, err := os.ReadFile(filePath); err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", filePath, err)
	}

	return Config{}, nil
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

	if _, err := file.WriteString("{}\n"); err != nil {
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
