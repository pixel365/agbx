package image

import (
	"context"

	"charm.land/huh/v2/spinner"
)

func runWithSpinner(ctx context.Context, title string, action func(context.Context) error) error {
	return spinner.New().
		Title(title).
		Context(ctx).
		ActionWithErr(action).
		Run()
}
