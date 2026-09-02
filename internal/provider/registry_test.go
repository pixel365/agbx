package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const providerName = "claude"

type testProvider struct {
	name string
}

func (provider testProvider) Name() string {
	return provider.name
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
