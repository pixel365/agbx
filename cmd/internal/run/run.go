package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/networkproxy"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/project"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Build(context.Context, docker.BuildRequest) error
	Run(context.Context, docker.RunRequest) error
	Close() error
	HasImage(context.Context, string) (bool, error)
}

type DockerClientFunc func() (DockerClient, error)

type NetworkProxyConfigurationFunc func(config.Config, string) (*docker.NetworkProxy, error)

const (
	dataHomeEnvironmentVariable = "XDG_DATA_HOME"
	workspaceDirectory          = "/workspace"
)

func NewProviderCommand(
	newDockerClient DockerClientFunc,
	selectedProvider provider.Provider,
) *cobra.Command {
	help := provider.HelpFor(selectedProvider)

	return &cobra.Command{
		Use:                selectedProvider.Name() + " [arguments...]",
		Short:              help.Short,
		Long:               help.Long,
		Example:            help.Example,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunProvider(
				cmd,
				args,
				newDockerClient,
				selectedProvider,
				networkProxyConfiguration,
			)
		},
	}
}

func RunProvider(
	cmd *cobra.Command,
	args []string,
	newDockerClient DockerClientFunc,
	selectedProvider provider.Provider,
	configureNetworkProxy NetworkProxyConfigurationFunc,
) error {
	loadedConfig, err := commandconfig.LoadWithPath(cmd)
	if err != nil {
		return err
	}
	configuration := loadedConfig.Configuration
	mounts, err := configuration.MountsForProvider(selectedProvider.Name())
	if err != nil {
		return fmt.Errorf("get mounts for provider %q: %w", selectedProvider.Name(), err)
	}
	recipe, err := provider.BuildRecipeFor(selectedProvider, configuration)
	if err != nil {
		return fmt.Errorf("create build recipe for provider %q: %w", selectedProvider.Name(), err)
	}
	imageReference := recipe.PreparedImageReference(
		selectedProvider.Name(),
		configuration.Image,
	)
	command, err := selectedProvider.Command(args, mounts)
	if err != nil {
		return fmt.Errorf("create command for provider %q: %w", selectedProvider.Name(), err)
	}

	dockerClient, err := newDockerClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = dockerClient.Close()
	}()

	if err := ensureProviderImage(
		cmd.Context(),
		dockerClient,
		configuration.Image,
		recipe,
		imageReference,
		selectedProvider.Name(),
		cmd.OutOrStdout(),
	); err != nil {
		return err
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	containerWorkspaceDirectory, err := projectWorkspaceDirectory(loadedConfig.FilePath)
	if err != nil {
		return err
	}
	stateDirectory, err := providerStateDirectory(selectedProvider.Name())
	if err != nil {
		return err
	}
	containerUser, err := currentUserIdentity()
	if err != nil {
		return err
	}
	networkProxy, err := configureNetworkProxy(configuration, selectedProvider.Name())
	if err != nil {
		return err
	}

	runErr := dockerClient.Run(cmd.Context(), docker.RunRequest{
		Command:            command,
		Image:              imageReference,
		Input:              cmd.InOrStdin(),
		Mounts:             dockerMounts(mounts),
		NetworkProxy:       networkProxy,
		Output:             cmd.OutOrStdout(),
		PullImage:          false,
		StateDirectory:     stateDirectory,
		User:               containerUser,
		WorkingDirectory:   workingDirectory,
		WorkspaceDirectory: containerWorkspaceDirectory,
	})
	if err := preparedcache.Record(
		loadedConfig.FilePath,
		selectedProvider.Name(),
		imageReference,
	); err != nil {
		return errors.Join(runErr, fmt.Errorf("record prepared image %q: %w", imageReference, err))
	}

	return runErr
}

func ensureProviderImage(
	ctx context.Context,
	dockerClient DockerClient,
	image config.Image,
	recipe provider.BuildRecipe,
	imageReference string,
	providerName string,
	output io.Writer,
) error {
	hasImage, err := dockerClient.HasImage(ctx, imageReference)
	if err != nil {
		return fmt.Errorf("check prepared image %q: %w", imageReference, err)
	}
	if hasImage {
		return nil
	}
	if err := dockerClient.Build(ctx, docker.BuildRequest{
		Dockerfile: recipe.Dockerfile,
		BuildArgs:  recipe.BuildArgs,
		Labels:     recipe.PreparedImageLabels(providerName, image),
		Output:     output,
		Tag:        imageReference,
	}); err != nil {
		return fmt.Errorf("build provider %q: %w", providerName, err)
	}
	_, err = fmt.Fprintf(output, "Prepared provider image: %s\n", imageReference)

	return err
}

func networkProxyConfiguration(
	configuration config.Config,
	providerName string,
) (*docker.NetworkProxy, error) {
	policy := configuration.NetworkPolicyForProvider(providerName)
	if configuration.Network.Audit == nil && policy == nil {
		return nil, nil
	}

	settings, err := networkproxy.Setup(configuration.Network.Audit, policy)
	if err != nil {
		return nil, fmt.Errorf("set up network proxy: %w", err)
	}

	return &docker.NetworkProxy{
		CertificatePath:     settings.CertificatePath,
		LogDirectory:        settings.LogDirectory,
		PolicyScriptPath:    settings.PolicyScriptPath,
		ProxyConfigPath:     settings.ProxyConfigPath,
		RedactionScriptPath: settings.RedactionScriptPath,
	}, nil
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

func projectWorkspaceDirectory(configFile string) (string, error) {
	projectIdentifier, err := project.ID(configFile)
	if err != nil {
		return "", err
	}

	return path.Join(workspaceDirectory, projectIdentifier), nil
}

func isProviderNamePathComponent(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		!strings.ContainsAny(name, "/\\")
}
