package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
)

const (
	baseImageArgument  = "BASE_IMAGE"
	imageName          = "example/image"
	imageTag           = "1.0"
	imageDigest        = "sha256:abc"
	baseImageReference = imageName + ":" + imageTag
	exampleDockerfile  = "FROM " + baseImageReference + "@" + imageDigest
	providerName       = "claude"
)

type testProvider struct {
	name string
}

func (provider testProvider) Name() string {
	return provider.name
}

func (testProvider) BuildRecipe(config.Image) (BuildRecipe, error) {
	return BuildRecipe{}, nil
}

func (testProvider) Command([]string, []config.Mount) ([]string, error) {
	return nil, nil
}

func TestRegistryLooksUpRegisteredProvider(t *testing.T) {
	registry := NewRegistry()
	want := testProvider{name: providerName}
	require.NoError(t, registry.Register(want))

	got, err := registry.Lookup(providerName)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRegistryRejectsDuplicateProviderName(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(testProvider{name: providerName}))

	err := registry.Register(testProvider{name: providerName})

	assert.ErrorIs(t, err, ErrDuplicateName)
}

func TestRegistryRejectsProviderWithoutName(t *testing.T) {
	err := NewRegistry().Register(testProvider{})

	assert.ErrorIs(t, err, ErrInvalidName)
}

func TestRegistryReturnsNotFoundForUnknownProvider(t *testing.T) {
	provider, err := NewRegistry().Lookup(providerName)

	assert.Nil(t, provider)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRegistryListsProvidersByName(t *testing.T) {
	registry := NewRegistry()
	firstProvider := testProvider{name: "codex"}
	secondProvider := testProvider{name: providerName}
	require.NoError(t, registry.Register(firstProvider))
	require.NoError(t, registry.Register(secondProvider))

	assert.Equal(t, []Provider{secondProvider, firstProvider}, registry.All())
}

func TestBuildRecipePreparedImageReference(t *testing.T) {
	image := config.Image{Name: imageName, Tag: imageTag, Digest: imageDigest}
	recipe := BuildRecipe{Dockerfile: exampleDockerfile}

	firstReference := recipe.PreparedImageReference(providerName, image)
	secondReference := recipe.PreparedImageReference(providerName, image)

	assert.Equal(t, firstReference, secondReference)
	assert.Contains(t, firstReference, preparedImageRepository+"-"+providerName+":")
	assert.NotEqual(t, firstReference, recipe.PreparedImageReference("codex", image))
	assert.NotEqual(
		t,
		firstReference,
		BuildRecipe{
			Dockerfile: "FROM " + imageName + ":2.0",
		}.PreparedImageReference(
			providerName,
			image,
		),
	)
	assert.NotEqual(
		t,
		firstReference,
		BuildRecipe{
			Dockerfile: exampleDockerfile,
			BuildArgs:  map[string]string{baseImageArgument: imageName + ":2.0"},
		}.PreparedImageReference(providerName, image),
	)
	assert.Equal(
		t,
		BuildRecipe{
			Dockerfile: exampleDockerfile,
			BuildArgs: map[string]string{
				baseImageArgument: baseImageReference,
				"VERSION":         imageTag,
			},
		}.PreparedImageReference(providerName, image),
		BuildRecipe{
			Dockerfile: exampleDockerfile,
			BuildArgs: map[string]string{
				"VERSION":         imageTag,
				baseImageArgument: baseImageReference,
			},
		}.PreparedImageReference(providerName, image),
	)
}

func TestNewBuildRecipeAddsRuntimeDockerfile(t *testing.T) {
	image := config.Image{Name: imageName, Tag: imageTag, Digest: imageDigest}
	providerDockerfile := "RUN install provider"

	recipe := NewBuildRecipe(image, providerDockerfile)

	assert.Equal(t, runtimeDockerfile+"\n"+providerDockerfile, recipe.Dockerfile)
	assert.Equal(t, map[string]string{baseImageBuildArg: image.Reference()}, recipe.BuildArgs)
}

func TestNewBuildRecipeAddsProviderPackages(t *testing.T) {
	image := config.Image{Name: imageName, Tag: imageTag, Digest: imageDigest}

	recipe := NewBuildRecipe(image, exampleDockerfile, "bubblewrap", "python3")

	assert.Equal(
		t,
		map[string]string{
			baseImageBuildArg:        image.Reference(),
			providerPackagesBuildArg: "bubblewrap python3",
		},
		recipe.BuildArgs,
	)
}
