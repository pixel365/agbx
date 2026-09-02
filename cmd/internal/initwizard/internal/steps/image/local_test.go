package image

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pixel365/agbx/internal/docker"
)

func TestFilterImages(t *testing.T) {
	images := []docker.Image{
		{Name: "golang", Tag: "1.27.0-alpine3.24"},
		{Name: "node", Tag: "24-alpine"},
	}

	assert.Equal(t, images, filterImages(images, ""))
	assert.Equal(t, []docker.Image{images[0]}, filterImages(images, "GOLANG"))
	assert.Equal(t, []docker.Image{images[1]}, filterImages(images, "24-alpine"))
	assert.Empty(t, filterImages(images, "python"))
}
