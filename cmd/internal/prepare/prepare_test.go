package prepare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareCommandRejectsUnsupportedProvider(t *testing.T) {
	cmd := NewPrepareCommand()
	cmd.SetArgs([]string{"claude"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.EqualError(t, err, "provider \"claude\" is not supported")
}

func TestPrepareCommandRequiresProvider(t *testing.T) {
	cmd := NewPrepareCommand()
	cmd.SetArgs(nil)

	require.Error(t, cmd.Execute())
}
