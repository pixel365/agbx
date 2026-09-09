package networklearn

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/run"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	learn "github.com/pixel365/agbx/internal/networklearn"
	"github.com/pixel365/agbx/internal/networkproxy"
	"github.com/pixel365/agbx/internal/provider"
)

const flowsFileName = "flows.har"

func NewNetworkCommand(
	newDockerClient run.DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	command := &cobra.Command{
		Use:   "network",
		Short: "Manage provider network access",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(NewLearnCommand(newDockerClient, providers))

	return command
}

func NewLearnCommand(
	newDockerClient run.DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	return &cobra.Command{
		Use:                "learn <provider> [arguments...]",
		Short:              "Observe network destinations used by a provider",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return learnProvider(cmd, args, newDockerClient, providers)
		},
	}
}

func learnProvider(
	cmd *cobra.Command,
	args []string,
	newDockerClient run.DockerClientFunc,
	providers *provider.Registry,
) (resultErr error) {
	selectedProvider, err := providers.Lookup(args[0])
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return fmt.Errorf("provider %q is not supported", args[0])
		}

		return err
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"Learning network destinations for provider %q. Exit the provider to generate a suggestion.\n",
		selectedProvider.Name(),
	); err != nil {
		return err
	}

	var settings networkproxy.Settings
	var cleanup func() error
	defer func() {
		if cleanup != nil {
			resultErr = errors.Join(resultErr, cleanup())
		}
	}()

	resultErr = run.RunProvider(
		cmd,
		args[1:],
		newDockerClient,
		selectedProvider,
		func(configuration config.Config, _ string) (*docker.NetworkProxy, error) {
			redaction := auditRedaction(configuration)
			var err error
			settings, cleanup, err = networkproxy.SetupTemporaryAudit(redaction)
			if err != nil {
				return nil, fmt.Errorf("set up temporary network audit: %w", err)
			}

			return &docker.NetworkProxy{
				CertificatePath:     settings.CertificatePath,
				LogDirectory:        settings.LogDirectory,
				PolicyScriptPath:    settings.PolicyScriptPath,
				ProxyConfigPath:     settings.ProxyConfigPath,
				RedactionScriptPath: settings.RedactionScriptPath,
			}, nil
		},
	)
	if settings.LogDirectory == "" {
		return resultErr
	}

	destinations, err := readDestinations(settings.LogDirectory)
	if err != nil {
		return errors.Join(resultErr, err)
	}

	return errors.Join(resultErr, writeSuggestion(cmd, selectedProvider.Name(), destinations))
}

func auditRedaction(configuration config.Config) config.AuditRedaction {
	if configuration.Network.Audit == nil {
		return config.AuditRedaction{}
	}

	return configuration.Network.Audit.Redact
}

func readDestinations(directory string) ([]string, error) {
	filePath := filepath.Join(directory, flowsFileName)
	// #nosec G304 -- The file is created in agbx's private temporary network audit directory.
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("read network learn flows %q: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	destinations, err := learn.Destinations(file)
	if err != nil {
		return nil, fmt.Errorf("parse network learn flows %q: %w", filePath, err)
	}

	return destinations, nil
}

func writeSuggestion(cmd *cobra.Command, providerName string, destinations []string) error {
	if len(destinations) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No network destinations were observed.")

		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Observed network destinations:"); err != nil {
		return err
	}
	for _, destination := range destinations {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", destination); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "\nSuggested provider policy:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"providers:\n  %s:\n    network:\n      allow:\n",
		providerName,
	); err != nil {
		return err
	}
	for _, destination := range destinations {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "        - %s\n", destination); err != nil {
			return err
		}
	}

	return nil
}
