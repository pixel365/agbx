package claude

import (
	_ "embed"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	name              = "claude"
	baseImageBuildArg = "BASE_IMAGE"
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
	return provider.BuildRecipe{
		Dockerfile: dockerfile,
		BuildArgs: map[string]string{
			baseImageBuildArg: image.Reference(),
		},
	}, nil
}

func (Provider) Command(args []string) ([]string, error) {
	return append([]string{"agbx-claude"}, args...), nil
}
