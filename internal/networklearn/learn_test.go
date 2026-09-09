package networklearn

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestinationsReturnsSortedUniqueValidHosts(t *testing.T) {
	flows := `{
  "log": {
    "entries": [
      {"request": {"url": "https://API.example.com/v1"}},
      {"request": {"url": "https://api.example.com/v2"}},
      {"request": {"url": "https://github.com/"}},
      {"request": {"url": "https://203.0.113.1/"}},
      {"request": {"url": "https://bad_host.example.com/"}}
    ]
  }
}`

	destinations, err := Destinations(strings.NewReader(flows))

	require.NoError(t, err)
	assert.Equal(t, []string{"api.example.com", "github.com"}, destinations)
}

func TestDestinationsRejectsInvalidArchive(t *testing.T) {
	_, err := Destinations(strings.NewReader("not JSON"))

	require.Error(t, err)
}
