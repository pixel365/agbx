package provider

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/pixel365/agbx/internal/config"
)

const (
	baseImageBuildArg        = "BASE_IMAGE"
	providerPackagesBuildArg = "PROVIDER_PACKAGES"
	preparedImageRepository  = "agbx/prepared"
)

//go:embed runtime/Dockerfile
var runtimeDockerfile string

type BuildRecipe struct {
	BuildArgs  map[string]string
	Dockerfile string
}

func NewBuildRecipe(
	image config.Image,
	providerDockerfile string,
	providerPackages ...string,
) BuildRecipe {
	buildArgs := map[string]string{
		baseImageBuildArg: image.Reference(),
	}
	if len(providerPackages) > 0 {
		buildArgs[providerPackagesBuildArg] = strings.Join(providerPackages, " ")
	}

	return BuildRecipe{
		Dockerfile: runtimeDockerfile + "\n" + providerDockerfile,
		BuildArgs:  buildArgs,
	}
}

func (recipe BuildRecipe) PreparedImageReference(providerName string, image config.Image) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(providerName))
	_, _ = hash.Write([]byte{'\n'})
	_, _ = hash.Write([]byte(image.Reference()))
	_, _ = hash.Write([]byte{'\n'})
	_, _ = hash.Write([]byte(recipe.Dockerfile))

	argumentNames := make([]string, 0, len(recipe.BuildArgs))
	for name := range recipe.BuildArgs {
		argumentNames = append(argumentNames, name)
	}
	sort.Strings(argumentNames)
	for _, name := range argumentNames {
		_, _ = hash.Write([]byte{'\n'})
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{'='})
		_, _ = hash.Write([]byte(recipe.BuildArgs[name]))
	}

	return preparedImageRepository + "-" + providerName + ":" + hex.EncodeToString(hash.Sum(nil))
}
