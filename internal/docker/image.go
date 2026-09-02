package docker

import (
	"context"
	"sort"

	"github.com/distribution/reference"
	mobyclient "github.com/moby/moby/client"
)

const imageSearchLimit = 25

type Image struct {
	Name   string
	Tag    string
	Digest string
}

type SearchResult struct {
	Name        string
	Description string
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	result, err := c.api.ImageList(ctx, mobyclient.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	images := make([]Image, 0)
	for i := range result.Items {
		for _, reference := range result.Items[i].RepoTags {
			image, ok := imageFromReference(reference, result.Items[i].RepoDigests)
			if ok {
				images = append(images, image)
			}
		}
	}
	sort.Slice(images, func(i, j int) bool {
		if images[i].Name == images[j].Name {
			return images[i].Tag < images[j].Tag
		}

		return images[i].Name < images[j].Name
	})

	return images, nil
}

func (c *Client) SearchImages(ctx context.Context, term string) ([]SearchResult, error) {
	result, err := c.api.ImageSearch(
		ctx,
		term,
		mobyclient.ImageSearchOptions{Limit: imageSearchLimit},
	)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		if item.Name == "" {
			continue
		}
		results = append(results, SearchResult{
			Name:        item.Name,
			Description: item.Description,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results, nil
}

func imageFromReference(imageReference string, repositoryDigests []string) (Image, bool) {
	named, err := reference.ParseNormalizedNamed(imageReference)
	if err != nil {
		return Image{}, false
	}
	tagged, ok := named.(reference.Tagged)
	if !ok {
		return Image{}, false
	}

	image := Image{
		Name: reference.FamiliarName(named),
		Tag:  tagged.Tag(),
	}
	if image.Tag == "latest" {
		return image, true
	}
	if digested, ok := named.(reference.Digested); ok {
		image.Digest = digested.Digest().String()

		return image, true
	}
	for _, repositoryDigest := range repositoryDigests {
		digestReference, err := reference.ParseNormalizedNamed(repositoryDigest)
		if err != nil || digestReference.Name() != named.Name() {
			continue
		}
		digested, ok := digestReference.(reference.Digested)
		if ok {
			image.Digest = digested.Digest().String()

			break
		}
	}

	return image, true
}
