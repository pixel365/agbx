package networkpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiHost       = "api.example.com"
	githubHost    = "github.com"
	telemetryHost = "telemetry.example.com"
)

func TestPolicyAllows(t *testing.T) {
	policy := Policy{
		Default: DefaultDeny,
		Allow:   []string{apiHost, "*.github.com"},
		Deny:    []string{"telemetry.github.com"},
	}

	require.NoError(t, policy.Validate())
	assert.True(t, policy.Allows(apiHost, 443))
	assert.True(t, policy.Allows("RAW.GITHUB.COM", 443))
	assert.False(t, policy.Allows("github.com", 443))
	assert.False(t, policy.Allows("telemetry.github.com", 443))
	assert.False(t, policy.Allows(apiHost, 8443))
	assert.False(t, policy.Allows("unlisted.example.com", 443))
}

func TestPolicyAllowsDefaultAllow(t *testing.T) {
	policy := Policy{Default: DefaultAllow, Deny: []string{"blocked.example.com"}}

	assert.True(t, policy.Allows("allowed.example.com", 443))
	assert.False(t, policy.Allows("blocked.example.com", 443))
	assert.False(t, policy.Allows("203.0.113.1", 443))
	assert.False(t, policy.Allows("2001:db8::1", 443))
}

func TestPolicyValidate(t *testing.T) {
	tests := map[string]Policy{
		"missing default":  {Allow: []string{apiHost}},
		"invalid default":  {Default: "block"},
		"URL":              {Default: DefaultDeny, Allow: []string{"https://example.com"}},
		"IP address":       {Default: DefaultDeny, Allow: []string{"127.0.0.1"}},
		"invalid wildcard": {Default: DefaultDeny, Allow: []string{"api.*.example.com"}},
		"duplicate":        {Default: DefaultDeny, Allow: []string{"example.com", "EXAMPLE.COM"}},
	}

	for name, policy := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, policy.Validate())
		})
	}
}

func TestPolicyWithAddsProviderRules(t *testing.T) {
	policy := Policy{Default: DefaultDeny, Allow: []string{githubHost}}
	resolved := policy.With(Additions{
		Allow: []string{apiHost},
		Deny:  []string{telemetryHost},
	})

	assert.Equal(t, []string{githubHost, apiHost}, resolved.Allow)
	assert.Equal(t, []string{telemetryHost}, resolved.Deny)
}
