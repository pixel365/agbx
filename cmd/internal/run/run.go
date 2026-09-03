package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Run(context.Context, docker.RunRequest) error
	Close() error
	HasImage(context.Context, string) (bool, error)
}

type DockerClientFunc func() (DockerClient, error)

const dataHomeEnvironmentVariable = "XDG_DATA_HOME"

func NewRunCommand(newDockerClient DockerClientFunc, providers *provider.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "run <provider> [arguments...]",
		Short: "Run a provider in the configured container",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProvider(cmd, args, newDockerClient, providers)
		},
	}
}

func runProvider(
	cmd *cobra.Command,
	args []string,
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) error {
	selectedProvider, err := providers.Lookup(args[0])
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return fmt.Errorf("provider %q is not supported", args[0])
		}

		return err
	}

	configuration, err := commandconfig.Load(cmd)
	if err != nil {
		return err
	}
	mounts, err := configuration.MountsForProvider(selectedProvider.Name())
	if err != nil {
		return fmt.Errorf("get mounts for provider %q: %w", args[0], err)
	}
	recipe, err := selectedProvider.BuildRecipe(configuration.Image)
	if err != nil {
		return fmt.Errorf("create build recipe for provider %q: %w", args[0], err)
	}
	imageReference := recipe.PreparedImageReference(
		selectedProvider.Name(),
		configuration.Image,
	)
	command, err := selectedProvider.Command(args[1:], mounts)
	if err != nil {
		return fmt.Errorf("create command for provider %q: %w", args[0], err)
	}

	dockerClient, err := newDockerClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = dockerClient.Close()
	}()

	hasImage, err := dockerClient.HasImage(cmd.Context(), imageReference)
	if err != nil {
		return fmt.Errorf("check prepared image %q: %w", imageReference, err)
	}
	if !hasImage {
		return fmt.Errorf(
			"provider %q is not prepared; run %q",
			args[0],
			"agbx prepare "+args[0],
		)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	stateDirectory, err := providerStateDirectory(selectedProvider.Name())
	if err != nil {
		return err
	}
	containerUser, err := currentUserIdentity()
	if err != nil {
		return err
	}

	return dockerClient.Run(cmd.Context(), docker.RunRequest{
		Command:          command,
		Image:            imageReference,
		Input:            cmd.InOrStdin(),
		Mounts:           dockerMounts(mounts),
		Output:           cmd.OutOrStdout(),
		PullImage:        false,
		StateDirectory:   stateDirectory,
		User:             containerUser,
		WorkingDirectory: workingDirectory,
	})
}

func dockerMounts(mounts []config.Mount) []docker.Mount {
	result := make([]docker.Mount, 0, len(mounts))
	for _, mount := range mounts {
		result = append(result, docker.Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			ReadOnly: mount.IsReadOnly(),
		})
	}

	return result
}

func providerStateDirectory(providerName string) (string, error) {
	if !isProviderNamePathComponent(providerName) {
		return "", fmt.Errorf("invalid provider name %q", providerName)
	}

	dataHome := os.Getenv(dataHomeEnvironmentVariable)
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home directory: %w", err)
		}

		dataHome = filepath.Join(home, ".local", "share")
	}

	directory := filepath.Join(dataHome, "agbx", "providers", providerName)
	directory = filepath.Clean(directory)

	// #nosec G703 -- providerName was validated as a single path component above.
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create provider state directory %q: %w", directory, err)
	}
	// #nosec G302 -- This directory stores provider authentication state and requires execute permission.
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", fmt.Errorf("set provider state directory permissions %q: %w", directory, err)
	}

	return directory, nil
}

func isProviderNamePathComponent(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		!strings.ContainsAny(name, "/\\")
}

func currentUserIdentity() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	if currentUser.Uid == "" || currentUser.Gid == "" {
		return "", errors.New("current user UID and GID are required")
	}

	return currentUser.Uid + ":" + currentUser.Gid, nil
}
