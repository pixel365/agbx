package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/docker"
)

const (
	checkCommand   = "check"
	configFlag     = "--config"
	validConfig    = "version: 1\nimage:\n  name: example/image\n  tag: latest\n"
	versionCommand = "version"
)

func TestRootCommandAllowsMissingDefaultConfigFile(t *testing.T) {
	changeWorkingDirectory(t, t.TempDir())

	var out bytes.Buffer
	cmd := newRootCommand(availableDockerClientFactory)
	cmd.SetArgs([]string{versionCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "dev\n", out.String())
}

func TestRootCommandLoadsExplicitConfigFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(validConfig), 0o600))

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetArgs([]string{configFlag, filePath, versionCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "dev\n", out.String())
}

func TestRootCommandRejectsMissingExplicitConfigFile(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "missing.yaml")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{configFlag, missingFile, versionCommand})
	cmd.SetErr(io.Discard)

	require.Error(t, cmd.Execute())
}

func TestRootCommandChecksDefaultConfigFile(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	var out bytes.Buffer
	cmd := newRootCommand(availableDockerClientFactory)
	cmd.SetArgs([]string{checkCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Configuration is valid.\nDocker daemon is available.\n", out.String())
}

func TestRootCommandChecksExplicitConfigFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(validConfig), 0o600))

	var out bytes.Buffer
	cmd := newRootCommand(availableDockerClientFactory)
	cmd.SetArgs([]string{configFlag, filePath, checkCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Configuration is valid.\nDocker daemon is available.\n", out.String())
}

func TestRootCommandRejectsMissingDefaultConfigFile(t *testing.T) {
	changeWorkingDirectory(t, t.TempDir())

	cmd := newRootCommand(availableDockerClientFactory)
	cmd.SetArgs([]string{checkCommand})
	cmd.SetErr(io.Discard)

	require.Error(t, cmd.Execute())
}

func TestRootCommandRejectsUnavailableDockerDaemon(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	cmd := newRootCommand(unavailableDockerClientFactory)
	cmd.SetArgs([]string{checkCommand})
	cmd.SetErr(io.Discard)

	require.ErrorIs(t, cmd.Execute(), errDockerUnavailable)
}

type availableDockerClient struct{}

func (availableDockerClient) Ping(context.Context) error {
	return nil
}

func (availableDockerClient) Close() error {
	return nil
}

func (availableDockerClient) Run(context.Context, docker.RunRequest) error {
	return nil
}

func (availableDockerClient) Build(context.Context, docker.BuildRequest) error {
	return nil
}

func availableDockerClientFactory() (dockerClient, error) {
	return availableDockerClient{}, nil
}

var errDockerUnavailable = errors.New("docker daemon is unavailable")

type unavailableDockerClient struct{}

func (unavailableDockerClient) Ping(context.Context) error {
	return errDockerUnavailable
}

func (unavailableDockerClient) Close() error {
	return nil
}

func (unavailableDockerClient) Run(context.Context, docker.RunRequest) error {
	return nil
}

func (unavailableDockerClient) Build(context.Context, docker.BuildRequest) error {
	return nil
}

func unavailableDockerClientFactory() (dockerClient, error) {
	return unavailableDockerClient{}, nil
}

func changeWorkingDirectory(t *testing.T, directory string) {
	t.Helper()

	previousDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(directory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previousDirectory))
	})
}
