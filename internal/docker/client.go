package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	mobyclient "github.com/moby/moby/client"
)

const (
	dockerConfigEnvironmentVariable  = "DOCKER_CONFIG"
	dockerContextEnvironmentVariable = "DOCKER_CONTEXT"
	defaultDockerContext             = "default"
)

type Client struct {
	api *mobyclient.Client
}

func NewClient() (*Client, error) {
	options := []mobyclient.Opt{mobyclient.FromEnv}
	if os.Getenv(mobyclient.EnvOverrideHost) == "" {
		host, err := currentContextHost()
		if err != nil {
			return nil, err
		}
		if host != "" {
			options = append(options, mobyclient.WithHost(host))
		}
	}

	api, err := mobyclient.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create Docker API client: %w", err)
	}

	return &Client{api: api}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.api.Ping(ctx, mobyclient.PingOptions{NegotiateAPIVersion: true}); err != nil {
		return fmt.Errorf("ping Docker API: %w", err)
	}

	return nil
}

func (c *Client) Close() error {
	return c.api.Close()
}

func currentContextHost() (string, error) {
	configDirectory, err := dockerConfigDirectory()
	if err != nil {
		return "", err
	}

	contextName := os.Getenv(dockerContextEnvironmentVariable)
	if contextName == "" {
		contextName, err = configuredContextName(configDirectory)
		if err != nil {
			return "", err
		}
	}
	if contextName == "" || contextName == defaultDockerContext {
		return "", nil
	}

	return contextHost(configDirectory, contextName)
}

func dockerConfigDirectory() (string, error) {
	if directory := os.Getenv(dockerConfigEnvironmentVariable); directory != "" {
		return directory, nil
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory for Docker config: %w", err)
	}

	return filepath.Join(homeDirectory, ".docker"), nil
}

func configuredContextName(configDirectory string) (string, error) {
	configFile := filepath.Join(configDirectory, "config.json")
	// #nosec G304 -- Docker CLI config is read from its standard user configuration directory.
	contents, err := os.ReadFile(configFile)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read Docker config file %q: %w", configFile, err)
	}

	var config struct {
		CurrentContext string `json:"currentContext"`
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		return "", fmt.Errorf("parse Docker config file %q: %w", configFile, err)
	}

	return config.CurrentContext, nil
}

func contextHost(configDirectory string, contextName string) (string, error) {
	contextHash := sha256.Sum256([]byte(contextName))
	metadataFile := filepath.Join(
		configDirectory,
		"contexts",
		"meta",
		hex.EncodeToString(contextHash[:]),
		"meta.json",
	)
	metadataFile = filepath.Clean(metadataFile)
	// #nosec G304 -- Docker context metadata is read from the selected Docker CLI configuration directory.
	contents, err := os.ReadFile(metadataFile)
	if err != nil {
		return "", fmt.Errorf("read Docker context %q: %w", contextName, err)
	}

	var metadata struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return "", fmt.Errorf("parse Docker context %q: %w", contextName, err)
	}

	dockerEndpoint, ok := metadata.Endpoints["docker"]
	if !ok || dockerEndpoint.Host == "" {
		return "", fmt.Errorf("docker context %q has no Docker endpoint", contextName)
	}

	return dockerEndpoint.Host, nil
}
