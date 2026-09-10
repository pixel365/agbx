package cache

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/provider"
)

func NewPruneCommand(
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	var apply bool

	command := &cobra.Command{
		Use:   "prune",
		Short: "Remove unused historical prepared images",
		Long: "List historical prepared images that are no longer current for any known project. " +
			"Without --apply, this command only shows removal candidates. Unattributed images " +
			"are never removed automatically.",
		Example: "  agbx cache prune\n" +
			"  agbx cache prune --apply",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return pruneImages(cmd, newDockerClient, providers, apply)
		},
	}
	command.Flags().BoolVar(&apply, "apply", false, "Remove the listed historical images")

	return command
}

func pruneImages(
	cmd *cobra.Command,
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
	apply bool,
) error {
	loadedConfig, err := commandconfig.LoadWithPath(cmd)
	if err != nil {
		return err
	}
	currentImages, err := currentImageReferences(loadedConfig.Configuration, providers)
	if err != nil {
		return err
	}
	references, err := preparedcache.AllImageReferences()
	if err != nil {
		return fmt.Errorf("load prepared image indexes: %w", err)
	}
	for imageReference := range currentImages {
		references.Current[imageReference] = struct{}{}
	}

	dockerClient, err := newDockerClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = dockerClient.Close()
	}()
	images, err := dockerClient.ListPreparedImages(cmd.Context())
	if err != nil {
		return fmt.Errorf("list prepared images: %w", err)
	}
	candidates := pruneCandidates(images, references)
	if err := writePruneCandidates(
		cmd.OutOrStdout(),
		loadedConfig.FilePath,
		candidates,
		apply,
	); err != nil {
		return err
	}
	if !apply || len(candidates) == 0 {
		return nil
	}

	return removeCandidates(cmd.Context(), dockerClient, cmd.OutOrStdout(), candidates)
}

func pruneCandidates(
	images []docker.PreparedImage,
	references preparedcache.ImageReferences,
) []docker.PreparedImage {
	candidates := make([]docker.PreparedImage, 0)
	for _, image := range images {
		if _, current := references.Current[image.Reference]; current {
			continue
		}
		if _, historical := references.Historical[image.Reference]; historical {
			candidates = append(candidates, image)
		}
	}

	return candidates
}

func writePruneCandidates(
	output io.Writer,
	configFile string,
	candidates []docker.PreparedImage,
	apply bool,
) error {
	if _, err := fmt.Fprintf(output, "Project: %s\n", configFile); err != nil {
		return err
	}
	if len(candidates) == 0 {
		_, err := fmt.Fprintln(output, "No unused historical prepared images were found.")

		return err
	}
	if _, err := fmt.Fprintf(
		output,
		"Historical images eligible for removal: %d\n\n",
		len(candidates),
	); err != nil {
		return err
	}
	images := make([]listedImage, 0, len(candidates))
	for _, candidate := range candidates {
		images = append(images, listedImage{Image: candidate, State: "historical"})
	}
	if err := writeImageTable(output, images); err != nil {
		return err
	}
	if apply {
		return nil
	}
	_, err := fmt.Fprintln(output, "\nRun \"agbx cache prune --apply\" to remove these images.")

	return err
}

func removeCandidates(
	ctx context.Context,
	dockerClient DockerClient,
	output io.Writer,
	candidates []docker.PreparedImage,
) error {
	var resultErr error
	removed := 0
	for _, candidate := range candidates {
		if err := dockerClient.RemovePreparedImage(ctx, candidate); err != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("remove prepared image %q: %w", candidate.Reference, err),
			)

			continue
		}
		removed++
	}
	if _, err := fmt.Fprintf(output, "Removed prepared images: %d\n", removed); err != nil {
		resultErr = errors.Join(resultErr, err)
	}

	return resultErr
}
