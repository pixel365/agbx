package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/cache"
	"github.com/pixel365/agbx/cmd/internal/check"
	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/cmd/internal/initcommand"
	"github.com/pixel365/agbx/cmd/internal/networklearn"
	"github.com/pixel365/agbx/cmd/internal/prepare"
	"github.com/pixel365/agbx/cmd/internal/run"
	"github.com/pixel365/agbx/cmd/internal/version"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
	"github.com/pixel365/agbx/internal/provider/claude"
	"github.com/pixel365/agbx/internal/provider/codex"
)

type dockerClient interface {
	cache.DockerClient
	check.DockerClient
	prepare.DockerClient
	run.DockerClient
}

type dockerClientFunc func() (dockerClient, error)

const (
	gettingStartedGroup       = "getting-started"
	providerEnvironmentsGroup = "provider-environments"
	networkGroup              = "network"
	providersGroup            = "providers"
	informationGroup          = "information"
)

func NewRootCommand() *cobra.Command {
	return newRootCommand(newDockerClient)
}

func newRootCommand(newDockerClient dockerClientFunc) *cobra.Command {
	var configFile string
	providers := mustProviderRegistry()

	cmd := &cobra.Command{
		Use:   "agbx",
		Short: "Run coding agents in isolated project environments",
		Long: "AGBX prepares reproducible Docker environments for coding agents. It mounts the " +
			"current project into an isolated container and can control mounts, network access, " +
			"and provider state.",
		Example: "  agbx init\n" +
			"  agbx check --verbose\n" +
			"  agbx claude\n" +
			"  agbx --config /path/to/.agbx.yaml codex",
	}
	cmd.AddGroup(
		&cobra.Group{ID: gettingStartedGroup, Title: "Getting started:"},
		&cobra.Group{ID: providerEnvironmentsGroup, Title: "Provider environments:"},
		&cobra.Group{ID: networkGroup, Title: "Network access:"},
		&cobra.Group{ID: providersGroup, Title: "Providers:"},
		&cobra.Group{ID: informationGroup, Title: "Information:"},
	)

	cmd.PersistentFlags().StringVar(
		&configFile,
		"config",
		"",
		"Path to the configuration file",
	)
	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		switch cmd.Name() {
		case "init", "check":
			return nil
		}
		if _, err := providers.Lookup(cmd.Name()); err == nil {
			return nil
		}

		_, err := commandconfig.Load(cmd)
		if errors.Is(err, config.ErrNotFound) {
			return nil
		}

		return err
	}

	initCommand := initcommand.NewInitCommand()
	initCommand.GroupID = gettingStartedGroup

	checkCommand := check.NewCheckCommand(func() (check.DockerClient, error) {
		return newDockerClient()
	}, providers)
	checkCommand.GroupID = gettingStartedGroup

	prepareCommand := prepare.NewPrepareCommand(func() (prepare.DockerClient, error) {
		return newDockerClient()
	}, providers)
	prepareCommand.GroupID = providerEnvironmentsGroup

	cacheCommand := cache.NewCacheCommand(func() (cache.DockerClient, error) {
		return newDockerClient()
	}, providers)
	cacheCommand.GroupID = providerEnvironmentsGroup

	networkCommand := networklearn.NewNetworkCommand(func() (run.DockerClient, error) {
		return newDockerClient()
	}, providers)
	networkCommand.GroupID = networkGroup

	versionCommand := version.NewVersionCommand()
	versionCommand.GroupID = informationGroup

	cmd.AddCommand(
		initCommand,
		prepareCommand,
		versionCommand,
		checkCommand,
		cacheCommand,
		networkCommand,
	)
	for _, registeredProvider := range providers.All() {
		providerCommand := run.NewProviderCommand(func() (run.DockerClient, error) {
			return newDockerClient()
		}, registeredProvider)
		providerCommand.GroupID = providersGroup
		cmd.AddCommand(providerCommand)
	}

	return cmd
}

func newDockerClient() (dockerClient, error) {
	return docker.NewClient()
}

func mustProviderRegistry() *provider.Registry {
	providers := provider.NewRegistry()
	if err := providers.Register(claude.New()); err != nil {
		panic(err)
	}
	if err := providers.Register(codex.New()); err != nil {
		panic(err)
	}

	return providers
}
