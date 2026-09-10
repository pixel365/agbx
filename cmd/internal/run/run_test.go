package run

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	auditLogDirectory       = "network-audit"
	exampleImageDigest      = "sha256:abc"
	exampleImageName        = "example/image"
	exampleImageTag         = "1.0"
	providerName            = "claude"
	sharedInstructionFile   = "AGENTS.md"
	sharedInstructionTarget = config.AdditionalMountDirectory + "/" + sharedInstructionFile
	instructionFile         = "CLAUDE.md"
	instructionMountTarget  = config.AdditionalMountDirectory + "/" + instructionFile
	validConfig             = "version: 1\nimage:\n  name: " + exampleImageName + "\n  tag: " +
		exampleImageTag + "\n  digest: " + exampleImageDigest + "\n"
)

func TestProviderCommandRunsConfiguredImage(t *testing.T) {
	directory := t.TempDir()
	stateHome := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, stateHome)
	configFile := filepath.Join(directory, ".agbx.yaml")
	require.NoError(
		t,
		os.WriteFile(configFile, []byte(validConfig), 0o600),
	)

	dockerClient := &recordingDockerClient{hasImage: true}
	cmd := NewProviderCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, testProvider{})
	cmd.SetArgs([]string{"--dangerously-skip-permissions"})

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(
		t,
		[]string{providerName, "--dangerously-skip-permissions"},
		dockerClient.request.Command,
	)
	expectedImage := config.Image{
		Name:   exampleImageName,
		Tag:    exampleImageTag,
		Digest: exampleImageDigest,
	}
	expectedRecipe := provider.BuildRecipe{Dockerfile: "FROM " + expectedImage.Reference()}
	assert.Equal(
		t,
		expectedRecipe.PreparedImageReference(providerName, expectedImage),
		dockerClient.request.Image,
	)
	assert.Equal(t, directory, dockerClient.request.WorkingDirectory)
	expectedWorkspaceDirectory, err := projectWorkspaceDirectory(configFile)
	require.NoError(t, err)
	assert.Equal(t, expectedWorkspaceDirectory, dockerClient.request.WorkspaceDirectory)
	expectedStateDirectory, err := providerStateDirectory(providerName)
	require.NoError(t, err)
	assert.Equal(t, expectedStateDirectory, dockerClient.request.StateDirectory)
	if runtime.GOOS == "windows" {
		assert.Empty(t, dockerClient.request.User)
	} else {
		assert.NotEmpty(t, dockerClient.request.User)
	}
	assert.NotNil(t, dockerClient.request.Input)
	assert.NotNil(t, dockerClient.request.Output)
	assert.False(t, dockerClient.request.PullImage)
	assert.True(t, dockerClient.closed)
}

func TestProviderCommandPreparesMissingImage(t *testing.T) {
	directory := t.TempDir()
	stateHome := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, stateHome)
	configFile := filepath.Join(directory, ".agbx.yaml")
	require.NoError(
		t,
		os.WriteFile(configFile, []byte(validConfig), 0o600),
	)

	dockerClient := &recordingDockerClient{}
	cmd := NewProviderCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, testProvider{})
	cmd.SetOut(io.Discard)

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	expectedImage := config.Image{
		Name:   exampleImageName,
		Tag:    exampleImageTag,
		Digest: exampleImageDigest,
	}
	expectedRecipe := provider.BuildRecipe{Dockerfile: "FROM " + expectedImage.Reference()}
	assert.Equal(t, expectedRecipe.Dockerfile, dockerClient.buildRequest.Dockerfile)
	assert.Equal(t, expectedRecipe.BuildArgs, dockerClient.buildRequest.BuildArgs)
	assert.Equal(
		t,
		expectedRecipe.PreparedImageLabels(providerName, expectedImage),
		dockerClient.buildRequest.Labels,
	)
	assert.Equal(
		t,
		expectedRecipe.PreparedImageReference(providerName, expectedImage),
		dockerClient.buildRequest.Tag,
	)
	assert.NotNil(t, dockerClient.buildRequest.Output)
	assert.Equal(t, dockerClient.buildRequest.Tag, dockerClient.request.Image)
	assert.True(t, dockerClient.closed)
}

func TestProviderCommandReturnsBuildError(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, t.TempDir())
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	buildErr := errors.New("build failed")
	dockerClient := &recordingDockerClient{buildErr: buildErr}
	cmd := NewProviderCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, testProvider{})

	err := cmd.ExecuteContext(t.Context())

	require.ErrorIs(t, err, buildErr)
	require.ErrorContains(t, err, "build provider \"claude\"")
	assert.Empty(t, dockerClient.request)
	assert.True(t, dockerClient.closed)
}

func TestProviderCommandPassesConfiguredMounts(t *testing.T) {
	directory := t.TempDir()
	stateHome := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, stateHome)
	sourcePath := filepath.Join(directory, instructionFile)
	sharedSourcePath := filepath.Join(directory, sharedInstructionFile)
	require.NoError(t, os.WriteFile(sourcePath, nil, 0o600))
	require.NoError(t, os.WriteFile(sharedSourcePath, nil, 0o600))
	contents := validConfig + "mounts:\n  - source: " + sharedInstructionFile +
		"\n    target: " + sharedInstructionTarget + "\nproviders:\n  " + providerName +
		":\n    mounts:\n      - source: " + instructionFile +
		"\n        target: " + instructionMountTarget + "\n"
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(contents), 0o600),
	)

	dockerClient := &recordingDockerClient{hasImage: true}
	cmd := NewProviderCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, testProvider{})

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(t, []docker.Mount{
		{
			Source:   sharedSourcePath,
			Target:   sharedInstructionTarget,
			ReadOnly: true,
		},
		{
			Source:   sourcePath,
			Target:   instructionMountTarget,
			ReadOnly: true,
		},
	}, dockerClient.request.Mounts)
}

func TestProviderCommandConfiguresNetworkProxyForAudit(t *testing.T) {
	directory := t.TempDir()
	stateHome := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, stateHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	contents := validConfig + "network:\n  audit:\n    log_directory: " + auditLogDirectory + "\n"
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(contents), 0o600),
	)

	dockerClient := &recordingDockerClient{hasImage: true}
	cmd := NewProviderCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, testProvider{})

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	require.NotNil(t, dockerClient.request.NetworkProxy)
	assert.Equal(
		t,
		filepath.Join(directory, auditLogDirectory),
		filepath.Dir(dockerClient.request.NetworkProxy.LogDirectory),
	)
}

func TestProviderCommandConfiguresNetworkPolicy(t *testing.T) {
	directory := t.TempDir()
	stateHome := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, stateHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	contents := validConfig + "network:\n  policy:\n    default: deny\n    allow:\n      - github.com\n" +
		"providers:\n  " + providerName + ":\n    network:\n      allow:\n        - api.example.com\n"
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(contents), 0o600),
	)

	dockerClient := &recordingDockerClient{hasImage: true}
	cmd := NewProviderCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, testProvider{})

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	require.NotNil(t, dockerClient.request.NetworkProxy)
	assert.Empty(t, dockerClient.request.NetworkProxy.LogDirectory)
	assert.NotEmpty(t, dockerClient.request.NetworkProxy.PolicyScriptPath)
}

func TestProviderStateDirectoryUsesXDGDataHome(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv(dataHomeEnvironmentVariable, dataHome)

	directory, err := providerStateDirectory(providerName)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dataHome, "agbx", "providers", providerName), directory)
	info, err := os.Stat(directory)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestProjectWorkspaceDirectoryIsScopedToConfigFile(t *testing.T) {
	firstConfigFile := filepath.Join(t.TempDir(), ".agbx.yaml")
	secondConfigFile := filepath.Join(t.TempDir(), ".agbx.yaml")
	require.NoError(t, os.WriteFile(firstConfigFile, []byte(validConfig), 0o600))
	require.NoError(t, os.WriteFile(secondConfigFile, []byte(validConfig), 0o600))

	firstDirectory, err := projectWorkspaceDirectory(firstConfigFile)
	require.NoError(t, err)
	secondDirectory, err := projectWorkspaceDirectory(secondConfigFile)
	require.NoError(t, err)

	assert.NotEqual(t, firstDirectory, secondDirectory)
}

func TestProviderStateDirectoryRejectsPath(t *testing.T) {
	for _, providerName := range []string{"../claude", "claude/provider", ".", ".."} {
		t.Run(providerName, func(t *testing.T) {
			_, err := providerStateDirectory(providerName)

			assert.Error(t, err)
		})
	}
}

type recordingDockerClient struct {
	buildErr     error
	buildRequest docker.BuildRequest
	request      docker.RunRequest
	hasImage     bool
	closed       bool
}

func (c *recordingDockerClient) HasImage(_ context.Context, _ string) (bool, error) {
	return c.hasImage, nil
}

func (c *recordingDockerClient) Build(_ context.Context, request docker.BuildRequest) error {
	c.buildRequest = request

	return c.buildErr
}

type testProvider struct{}

func (testProvider) Name() string {
	return providerName
}

func (testProvider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}, nil
}

func (testProvider) Command(args []string, _ []config.Mount) ([]string, error) {
	return append([]string{providerName}, args...), nil
}

func (c *recordingDockerClient) Run(_ context.Context, request docker.RunRequest) error {
	c.request = request

	return nil
}

func (c *recordingDockerClient) Close() error {
	c.closed = true

	return nil
}

func changeWorkingDirectory(t *testing.T, directory string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	previousDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(directory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previousDirectory))
	})
}
