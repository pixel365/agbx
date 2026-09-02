package prepare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/provider"
)

const providerName = "claude"

func TestPrepareCommandRejectsUnsupportedProvider(t *testing.T) {
	cmd := NewPrepareCommand(provider.NewRegistry())
	cmd.SetArgs([]string{providerName})

	err := cmd.Execute()

	require.Error(t, err)
	assert.EqualError(t, err, "provider \"claude\" is not supported")
}

func TestPrepareCommandRequiresProvider(t *testing.T) {
	cmd := NewPrepareCommand(provider.NewRegistry())
	cmd.SetArgs(nil)

	require.Error(t, cmd.Execute())
}

func TestPrepareCommandRejectsUnimplementedRegisteredProvider(t *testing.T) {
	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(testProvider{}))
	cmd := NewPrepareCommand(providers)
	cmd.SetArgs([]string{providerName})

	err := cmd.Execute()

	require.Error(t, err)
	assert.EqualError(t, err, "preparation for provider \"claude\" is not implemented")
}

type testProvider struct{}

func (testProvider) Name() string {
	return providerName
}
