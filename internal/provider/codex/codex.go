package codex

import (
	_ "embed"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	name          = "codex"
	loginCommand  = "login"
	logoutCommand = "logout"
	bubblewrap    = "bubblewrap"
)

//go:embed Dockerfile
var dockerfile string

type Provider struct{}

func New() Provider {
	return Provider{}
}

func (Provider) Name() string {
	return name
}

func (Provider) Help() provider.Help {
	return provider.Help{
		Short: "Run Codex in the configured container",
		Long: "Run Codex in the prepared project environment. Arguments are passed through " +
			"unchanged to Codex. Place agbx global flags before the provider name. Use agbx " +
			"help codex for launcher help, or agbx codex --help for Codex help inside the " +
			"container.",
		Example: "  agbx codex\n" +
			"  agbx codex \"Describe this project\"\n" +
			"  agbx codex login --device-auth",
	}
}

func (Provider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.NewBuildRecipe(image, dockerfile, bubblewrap), nil
}

func (Provider) Command(args []string, mounts []config.Mount) ([]string, error) {
	command := []string{"agbx-codex"}
	if isAuthenticationCommand(args) {
		return append(command, args...), nil
	}

	command = append(command, "--sandbox", "workspace-write")
	if len(mounts) > 0 {
		command = append(command, "--add-dir", config.AdditionalMountDirectory)
	}

	return append(command, args...), nil
}

func isAuthenticationCommand(args []string) bool {
	return len(args) > 0 && (args[0] == loginCommand || args[0] == logoutCommand)
}
