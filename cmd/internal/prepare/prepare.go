package prepare

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Build(context.Context, docker.BuildRequest) error
	Close() error
	HasImage(context.Context, string) (bool, error)
}

type DockerClientFunc func() (DockerClient, error)

func NewPrepareCommand(
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "prepare <provider>",
		Short: "Prepare a provider environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return prepareProvider(cmd, args[0], force, newDockerClient, providers)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Rebuild an existing provider image")

	return cmd
}

func prepareProvider(
	cmd *cobra.Command,
	providerName string,
	force bool,
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) error {
	selectedProvider, err := lookupProvider(providerName, providers)
	if err != nil {
		return err
	}
	loadedConfig, err := commandconfig.LoadWithPath(cmd)
	if err != nil {
		return err
	}
	recipe, err := provider.BuildRecipeFor(selectedProvider, loadedConfig.Configuration)
	if err != nil {
		return fmt.Errorf("create build recipe for provider %q: %w", providerName, err)
	}

	dockerClient, err := newDockerClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = dockerClient.Close()
	}()

	imageReference := recipe.PreparedImageReference(providerName, loadedConfig.Configuration.Image)
	alreadyPrepared, err := prepareImage(
		cmd,
		dockerClient,
		recipe,
		providerName,
		loadedConfig.Configuration.Image,
		imageReference,
		force,
	)
	if err != nil {
		return err
	}
	if err := preparedcache.Record(
		loadedConfig.FilePath,
		providerName,
		imageReference,
	); err != nil {
		return fmt.Errorf("record prepared image %q: %w", imageReference, err)
	}

	return writePrepareResult(cmd, imageReference, alreadyPrepared)
}

func lookupProvider(providerName string, providers *provider.Registry) (provider.Provider, error) {
	selectedProvider, err := providers.Lookup(providerName)
	if errors.Is(err, provider.ErrNotFound) {
		return nil, fmt.Errorf("provider %q is not supported", providerName)
	}

	return selectedProvider, err
}

func prepareImage(
	cmd *cobra.Command,
	dockerClient DockerClient,
	recipe provider.BuildRecipe,
	providerName string,
	image config.Image,
	imageReference string,
	force bool,
) (bool, error) {
	hasImage, err := dockerClient.HasImage(cmd.Context(), imageReference)
	if err != nil {
		return false, fmt.Errorf("check prepared image %q: %w", imageReference, err)
	}
	if hasImage && !force {
		return true, nil
	}
	if err := dockerClient.Build(cmd.Context(), docker.BuildRequest{
		Dockerfile: recipe.Dockerfile,
		BuildArgs:  recipe.BuildArgs,
		Labels:     recipe.PreparedImageLabels(providerName, image),
		Output:     cmd.OutOrStdout(),
		Tag:        imageReference,
	}); err != nil {
		return false, fmt.Errorf("build provider %q: %w", providerName, err)
	}

	return false, nil
}

func writePrepareResult(cmd *cobra.Command, imageReference string, alreadyPrepared bool) error {
	if alreadyPrepared {
		_, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Provider image is already prepared: %s\n",
			imageReference,
		)

		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Prepared provider image: %s\n", imageReference)

	return err
}
