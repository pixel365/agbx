package initwizard

import (
	"context"

	"github.com/pixel365/agbx/cmd/internal/initwizard/internal/steps/image"
	"github.com/pixel365/agbx/internal/config"
)

type Step interface {
	Run(context.Context, *config.Config) error
}

type Wizard struct {
	steps []Step
}

func New(steps ...Step) Wizard {
	return Wizard{steps: steps}
}

func Default() Wizard {
	return New(image.New())
}

func (w Wizard) Run(ctx context.Context) (config.Config, error) {
	configuration := config.New()
	for _, step := range w.steps {
		if err := step.Run(ctx, &configuration); err != nil {
			return config.Config{}, err
		}
	}
	if err := configuration.Validate(); err != nil {
		return config.Config{}, err
	}

	return configuration, nil
}
