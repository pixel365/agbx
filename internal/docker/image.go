package docker

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/distribution/reference"
	mobyclient "github.com/moby/moby/client"

	"github.com/pixel365/agbx/internal/preparedimage"
)

const imageSearchLimit = 25

type Image struct {
	Name   string
	Tag    string
	Digest string
}

type PreparedImage struct {
	CreatedAt time.Time
	ImageID   string
	Reference string
	Provider  string
	Size      int64
	Tagged    bool
	Legacy    bool
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

func (c *Client) ListPreparedImages(ctx context.Context) ([]PreparedImage, error) {
	result, err := c.api.ImageList(ctx, mobyclient.ImageListOptions{All: true})
	if err != nil {
		return nil, err
	}

	images := make([]PreparedImage, 0)
	for imageIndex := range result.Items {
		image := &result.Items[imageIndex]
		if providerName, imageReference, ok := preparedimage.FromLabels(image.Labels); ok {
			images = append(images, PreparedImage{
				CreatedAt: time.Unix(image.Created, 0),
				ImageID:   image.ID,
				Reference: imageReference,
				Provider:  providerName,
				Size:      image.Size,
				Tagged:    hasReference(image.RepoTags, imageReference),
			})

			continue
		}

		for _, imageReference := range image.RepoTags {
			providerName, ok := preparedimage.ParseReference(imageReference)
			if !ok {
				continue
			}
			images = append(images, PreparedImage{
				CreatedAt: time.Unix(image.Created, 0),
				ImageID:   image.ID,
				Reference: imageReference,
				Provider:  providerName,
				Size:      image.Size,
				Tagged:    true,
				Legacy:    true,
			})
		}
	}
	sort.Slice(images, func(i, j int) bool {
		if images[i].Provider == images[j].Provider {
			if images[i].Reference == images[j].Reference {
				return images[i].ImageID < images[j].ImageID
			}

			return images[i].Reference < images[j].Reference
		}

		return images[i].Provider < images[j].Provider
	})

	return images, nil
}

func (c *Client) RemovePreparedImage(ctx context.Context, image PreparedImage) error {
	target := image.Reference
	if !image.Tagged {
		target = image.ImageID
	}
	if _, err := c.api.ImageRemove(ctx, target, mobyclient.ImageRemoveOptions{}); err != nil {
		return err
	}

	return nil
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

func hasReference(references []string, imageReference string) bool {
	return slices.Contains(references, imageReference)
}
