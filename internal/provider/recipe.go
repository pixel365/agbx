package provider

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/pixel365/agbx/internal/config"
)

const preparedImageRepository = "agbx/prepared"

type BuildRecipe struct {
	Dockerfile string
}

func (recipe BuildRecipe) PreparedImageReference(providerName string, image config.Image) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(providerName))
	_, _ = hash.Write([]byte{'\n'})
	_, _ = hash.Write([]byte(image.Reference()))
	_, _ = hash.Write([]byte{'\n'})
	_, _ = hash.Write([]byte(recipe.Dockerfile))

	return preparedImageRepository + ":" + hex.EncodeToString(hash.Sum(nil))
}
