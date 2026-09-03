package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

const (
	defaultYAMLFileName = ".agbx.yaml"
	defaultYMLFileName  = ".agbx.yml"
	currentVersion      = 1

	AdditionalMountDirectory = "/agbx/mounts"
)

var ErrNotFound = errors.New("config file not found")

type Config struct {
	Image   Image   `yaml:"image"`
	Mounts  []Mount `yaml:"mounts,omitempty"`
	Version int     `yaml:"version"`
}

type Image struct {
	Name   string `yaml:"name"`
	Tag    string `yaml:"tag"`
	Digest string `yaml:"digest,omitempty"`
}

type Mount struct {
	ReadOnly *bool  `yaml:"read_only,omitempty"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
}

func (mount Mount) IsReadOnly() bool {
	return mount.ReadOnly == nil || *mount.ReadOnly
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

func (c Config) Validate() error {
	if c.Version == 0 {
		return errors.New("config version is required")
	}
	if c.Version != currentVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if strings.TrimSpace(c.Image.Name) == "" {
		return errors.New("config image name is required")
	}
	if strings.TrimSpace(c.Image.Tag) == "" {
		return errors.New("config image tag is required")
	}
	if err := c.validateMounts(); err != nil {
		return err
	}

	return nil
}

func (c Config) validateMounts() error {
	targets := make(map[string]int, len(c.Mounts))
	for index, mount := range c.Mounts {
		mountNumber := index + 1
		if strings.TrimSpace(mount.Source) == "" {
			return fmt.Errorf("config mount %d source is required", mountNumber)
		}
		if strings.TrimSpace(mount.Target) == "" {
			return fmt.Errorf("config mount %d target is required", mountNumber)
		}
		if !path.IsAbs(mount.Target) {
			return fmt.Errorf(
				"config mount %d target %q must be absolute",
				mountNumber,
				mount.Target,
			)
		}

		target := path.Clean(mount.Target)
		if !isAdditionalMountTarget(target) {
			return fmt.Errorf(
				"config mount %d target %q must be inside %q",
				mountNumber,
				target,
				AdditionalMountDirectory,
			)
		}
		if previousTarget, previousMount, found := overlappingMountTarget(target, targets); found {
			return fmt.Errorf(
				"config mount %d target %q overlaps config mount %d target %q",
				mountNumber,
				target,
				previousMount,
				previousTarget,
			)
		}

		targets[target] = mountNumber
	}

	return nil
}

func isAdditionalMountTarget(target string) bool {
	return target == AdditionalMountDirectory ||
		strings.HasPrefix(target, AdditionalMountDirectory+"/")
}

func overlappingMountTarget(target string, targets map[string]int) (string, int, bool) {
	for previousTarget, previousMount := range targets {
		if mountPathsOverlap(target, previousTarget) {
			return previousTarget, previousMount, true
		}
	}

	return "", 0, false
}

func mountPathsOverlap(first string, second string) bool {
	return first == "/" ||
		second == "/" ||
		first == second ||
		strings.HasPrefix(first, second+"/") ||
		strings.HasPrefix(second, first+"/")
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
	configurationDirectory, err := filepath.Abs(filepath.Dir(filePath))
	if err != nil {
		return Config{}, fmt.Errorf("get config directory for %q: %w", filePath, err)
	}
	if err := configuration.resolveMountSources(configurationDirectory); err != nil {
		return Config{}, fmt.Errorf("resolve config mounts in file %q: %w", filePath, err)
	}

	return configuration, nil
}

func (c *Config) resolveMountSources(directory string) error {
	for index := range c.Mounts {
		mount := &c.Mounts[index]
		source, err := expandEnvironmentVariables(mount.Source)
		if err != nil {
			return fmt.Errorf("config mount %d source %q: %w", index+1, mount.Source, err)
		}
		if !filepath.IsAbs(source) {
			source = filepath.Join(directory, source)
		}
		source = filepath.Clean(source)

		// #nosec G703 -- Mount source paths are intentionally resolved from the selected config file.
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("config mount %d source %q: %w", index+1, mount.Source, err)
		}

		mount.Source = source
	}

	return nil
}

func expandEnvironmentVariables(source string) (string, error) {
	var expanded strings.Builder
	for source != "" {
		variableStart := strings.IndexByte(source, '$')
		if variableStart == -1 {
			expanded.WriteString(source)

			break
		}

		expanded.WriteString(source[:variableStart])
		source = source[variableStart:]
		if !strings.HasPrefix(source, "${") {
			return "", errors.New("environment variables must use ${NAME} syntax")
		}
		variableEnd := strings.IndexByte(source, '}')
		if variableEnd == -1 {
			return "", errors.New("environment variable is missing closing brace")
		}

		name := source[2:variableEnd]
		if !isEnvironmentVariableName(name) {
			return "", fmt.Errorf("invalid environment variable name %q", name)
		}
		value, found := os.LookupEnv(name)
		if !found || value == "" {
			return "", fmt.Errorf("environment variable %q is not set", name)
		}

		expanded.WriteString(value)
		source = source[variableEnd+1:]
	}

	return expanded.String(), nil
}

func isEnvironmentVariableName(name string) bool {
	for index, character := range name {
		if character == '_' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			index > 0 && character >= '0' && character <= '9' {
			continue
		}

		return false
	}

	return name != ""
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
