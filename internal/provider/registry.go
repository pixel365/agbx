package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pixel365/agbx/internal/config"
)

var (
	ErrDuplicateName = errors.New("provider name is already registered")
	ErrInvalidName   = errors.New("provider name is required")
	ErrNotFound      = errors.New("provider not found")
)

type Provider interface {
	Name() string
	BuildRecipe(config.Image) (BuildRecipe, error)
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{make(map[string]Provider)}
}

func (registry *Registry) Register(provider Provider) error {
	name := provider.Name()
	if strings.TrimSpace(name) == "" {
		return ErrInvalidName
	}
	if _, found := registry.providers[name]; found {
		return fmt.Errorf("%w: %q", ErrDuplicateName, name)
	}

	registry.providers[name] = provider

	return nil
}

func (registry *Registry) Lookup(name string) (Provider, error) {
	provider, found := registry.providers[name]
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}

	return provider, nil
}
