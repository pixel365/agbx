package claude

import (
	_ "embed"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	name = "claude"
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
		Short: "Run Claude Code in the configured container",
		Long: "Run Claude Code in the prepared project environment. Arguments are passed " +
			"through unchanged to Claude Code. Place agbx global flags before the provider " +
			"name. Use agbx help claude for launcher help, or agbx claude --help for Claude " +
			"Code help inside the container.",
		Example: "  agbx claude\n" +
			"  agbx claude -p \"Review the current changes\"\n" +
			"  agbx --config /path/to/.agbx.yaml claude",
	}
}

func (Provider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.NewBuildRecipe(image, dockerfile), nil
}

func (Provider) Command(args []string, mounts []config.Mount) ([]string, error) {
	command := []string{"agbx-claude"}
	if len(mounts) > 0 {
		command = append(command, "--add-dir", config.AdditionalMountDirectory)
	}

	return append(command, args...), nil
}
