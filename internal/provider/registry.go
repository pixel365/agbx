package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pixel365/agbx/internal/config"
)

var (
	ErrDuplicateName = errors.New("provider name is already registered")
	ErrInvalidName   = errors.New("provider name is required")
	ErrNotFound      = errors.New("provider not found")
)

type Provider interface {
	BuildRecipe(config.Image) (BuildRecipe, error)
	Command([]string, []config.Mount) ([]string, error)
	Name() string
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

func (registry *Registry) All() []Provider {
	providers := make([]Provider, 0, len(registry.providers))
	for _, provider := range registry.providers {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(first int, second int) bool {
		return providers[first].Name() < providers[second].Name()
	})

	return providers
}
