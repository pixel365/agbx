package docker

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/charmbracelet/x/term"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardTerminalResizeSkipsNonTerminalInput(t *testing.T) {
	client := &Client{}

	stop, err := client.forwardTerminalResize(
		context.Background(),
		"container",
		bytes.NewReader(nil),
	)

	require.NoError(t, err)
	stop()
}

func TestContainerMounts(t *testing.T) {
	request := RunRequest{
		Mounts: []Mount{{
			Source:   "/host/CLAUDE.md",
			Target:   "/agbx/CLAUDE.md",
			ReadOnly: true,
		}},
		StateDirectory:     "/host/state",
		WorkingDirectory:   "/host/workspace",
		WorkspaceDirectory: "/workspace/project",
	}

	assert.Equal(t, []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: "/host/workspace",
			Target: "/workspace/project",
		},
		{
			Type:   mount.TypeBind,
			Source: "/host/state",
			Target: homeDirectory,
		},
		{
			Type:     mount.TypeBind,
			Source:   "/host/CLAUDE.md",
			Target:   "/agbx/CLAUDE.md",
			ReadOnly: true,
		},
	}, containerMounts(request))
}

func TestMakeRawInputRestoresTerminal(t *testing.T) {
	input, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("open pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		require.NoError(t, input.Close())
	})
	if !term.IsTerminal(input.Fd()) {
		t.Skip("pseudo-terminal is not supported")
	}

	initialState, err := term.GetState(input.Fd())
	require.NoError(t, err)
	restore, err := makeRawInput(input)
	require.NoError(t, err)

	rawState, err := term.GetState(input.Fd())
	require.NoError(t, err)
	assert.NotEqual(t, initialState, rawState)

	require.NoError(t, restore())
	restoredState, err := term.GetState(input.Fd())
	require.NoError(t, err)
	assert.Equal(t, initialState, restoredState)
}
