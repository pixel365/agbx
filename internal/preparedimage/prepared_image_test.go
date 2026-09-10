package preparedimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	imageProvider  = "claude"
	imageReference = "agbx/prepared-claude:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestFromLabelsReturnsPreparedImage(t *testing.T) {
	provider, reference, ok := FromLabels(Labels(imageProvider, imageReference))

	assert.True(t, ok)
	assert.Equal(t, imageProvider, provider)
	assert.Equal(t, imageReference, reference)
}

func TestParseReferenceRejectsNonPreparedImage(t *testing.T) {
	_, ok := ParseReference("example/image:latest")

	assert.False(t, ok)
}
