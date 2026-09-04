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
