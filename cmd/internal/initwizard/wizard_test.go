package initwizard

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
)

func TestWizardRunsAllSteps(t *testing.T) {
	wizard := New(
		stepFunc(func(_ context.Context, configuration *config.Config) error {
			configuration.Image.Name = "example/image"

			return nil
		}),
		stepFunc(func(_ context.Context, configuration *config.Config) error {
			configuration.Image.Tag = "latest"

			return nil
		}),
	)

	configuration, err := wizard.Run(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "example/image", configuration.Image.Name)
	assert.Equal(t, "latest", configuration.Image.Tag)
}

func TestWizardStopsWhenStepReturnsError(t *testing.T) {
	errStepFailed := errors.New("step failed")
	wizard := New(stepFunc(func(context.Context, *config.Config) error {
		return errStepFailed
	}))

	_, err := wizard.Run(t.Context())

	assert.ErrorIs(t, err, errStepFailed)
}

type stepFunc func(context.Context, *config.Config) error

func (f stepFunc) Run(ctx context.Context, configuration *config.Config) error {
	return f(ctx, configuration)
}
