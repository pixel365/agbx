package docker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mobyclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientHasImageReturnsFalseForMissingImage(t *testing.T) {
	dockerServer := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		http.Error(response, "No such image", http.StatusNotFound)
	}))
	t.Cleanup(dockerServer.Close)

	api, err := mobyclient.New(mobyclient.WithHost(dockerServer.URL))
	require.NoError(t, err)
	client := Client{api: api}

	hasImage, err := client.HasImage(t.Context(), "missing")

	require.NoError(t, err)
	assert.False(t, hasImage)
}
