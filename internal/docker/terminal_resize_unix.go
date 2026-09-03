//go:build !windows

package docker

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

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

	resizeContext, cancel := context.WithCancel(ctx)
	resizeSignals := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(resizeSignals, syscall.SIGWINCH)
	go func() {
		defer close(done)
		defer signal.Stop(resizeSignals)

		for {
			select {
			case <-resizeContext.Done():
				return
			case <-resizeSignals:
				_ = c.resizeTerminal(resizeContext, containerID, terminal)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}, nil
}
