//go:build windows

package docker

import (
	"context"
	"io"

	"github.com/charmbracelet/x/term"
)

func (c *Client) forwardTerminalResize(
	ctx context.Context,
	containerID string,
	input io.Reader,
) (func(), error) {
	terminal, ok := input.(terminalInput)
	if !ok || !term.IsTerminal(terminal.Fd()) {
		return func() {}, nil
	}
	if err := c.resizeTerminal(ctx, containerID, terminal); err != nil {
		return nil, err
	}

	return func() {}, nil
}
