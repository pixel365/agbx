package preparedimage

import (
	"encoding/hex"
	"strings"

	"github.com/distribution/reference"
)

const (
	managedLabel   = "io.agbx.managed"
	kindLabel      = "io.agbx.kind"
	providerLabel  = "io.agbx.provider"
	referenceLabel = "io.agbx.reference"

	managedValue = "true"
	kindValue    = "prepared-image"

	repositoryPrefix = "agbx/prepared-"
	hashLength       = 64
)

func Labels(providerName string, imageReference string) map[string]string {
	return map[string]string{
		managedLabel:   managedValue,
		kindLabel:      kindValue,
		providerLabel:  providerName,
		referenceLabel: imageReference,
	}
}

func FromLabels(labels map[string]string) (providerName string, imageReference string, ok bool) {
	if labels[managedLabel] != managedValue || labels[kindLabel] != kindValue {
		return "", "", false
	}

	providerName = labels[providerLabel]
	imageReference = labels[referenceLabel]
	parsedProvider, ok := ParseReference(imageReference)
	if !ok || parsedProvider != providerName {
		return "", "", false
	}

	return providerName, imageReference, true
}

func ParseReference(imageReference string) (string, bool) {
	named, err := reference.ParseNormalizedNamed(imageReference)
	if err != nil {
		return "", false
	}
	tagged, ok := named.(reference.Tagged)
	if !ok {
		return "", false
	}

	repository := reference.FamiliarName(named)
	if !strings.HasPrefix(repository, repositoryPrefix) {
		return "", false
	}
	providerName := strings.TrimPrefix(repository, repositoryPrefix)
	if providerName == "" {
		return "", false
	}

	tag := tagged.Tag()
	if len(tag) != hashLength {
		return "", false
	}
	if _, err := hex.DecodeString(tag); err != nil {
		return "", false
	}

	return providerName, true
}
