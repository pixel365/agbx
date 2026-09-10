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

type Help struct {
	Example string
	Long    string
	Short   string
}

type HelpProvider interface {
	Help() Help
}

func HelpFor(selectedProvider Provider) Help {
	if documentedProvider, ok := selectedProvider.(HelpProvider); ok {
		return documentedProvider.Help()
	}

	providerName := selectedProvider.Name()

	return Help{
		Short: "Run " + providerName + " in the configured container",
		Long: "Run the provider in the prepared project environment. Arguments are passed " +
			"through to the provider without being parsed by agbx. Place global flags before " +
			"the provider name, for example: agbx --config /path/to/.agbx.yaml " + providerName + ".",
		Example: "  agbx " + providerName,
	}
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
