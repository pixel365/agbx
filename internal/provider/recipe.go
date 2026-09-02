package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/pixel365/agbx/internal/config"
)

const preparedImageRepository = "agbx/prepared"

type BuildRecipe struct {
	BuildArgs  map[string]string
	Dockerfile string
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

	return preparedImageRepository + ":" + hex.EncodeToString(hash.Sum(nil))
}
