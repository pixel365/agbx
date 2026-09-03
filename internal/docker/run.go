package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/x/term"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	mobyclient "github.com/moby/moby/client"
)

const (
	workspaceDirectory = "/workspace"
	homeDirectory      = "/home/agbx"
)

type RunRequest struct {
	Input            io.Reader
	Output           io.Writer
	Image            string
	Mounts           []Mount
	StateDirectory   string
	User             string
	WorkingDirectory string
	Command          []string
	PullImage        bool
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

func (c *Client) Run(ctx context.Context, request RunRequest) (runErr error) {
	if request.PullImage {
		if err := c.pullImage(ctx, request.Image); err != nil {
			return err
		}
	}

	createdContainer, err := c.createContainer(ctx, request)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = c.api.ContainerRemove(
			context.Background(),
			createdContainer.ID,
			mobyclient.ContainerRemoveOptions{Force: true},
		)
	}()

	attached, err := c.attachContainer(ctx, createdContainer.ID)
	if err != nil {
		return err
	}
	defer attached.Close()
	restoreInput, err := makeRawInput(request.Input)
	if err != nil {
		return fmt.Errorf("set terminal input to raw mode: %w", err)
	}
	defer func() {
		if err := restoreInput(); err != nil && runErr == nil {
			runErr = fmt.Errorf("restore terminal input: %w", err)
		}
	}()

	return c.runContainer(ctx, createdContainer.ID, attached, request)
}

func (c *Client) pullImage(ctx context.Context, image string) error {
	pullResponse, err := c.api.ImagePull(ctx, image, mobyclient.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}
	if err := pullResponse.Wait(ctx); err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}

	return nil
}

func (c *Client) createContainer(
	ctx context.Context,
	request RunRequest,
) (mobyclient.ContainerCreateResult, error) {
	createdContainer, err := c.api.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config: &container.Config{
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          request.Command,
			Env:          []string{"HOME=" + homeDirectory},
			Image:        request.Image,
			OpenStdin:    true,
			Tty:          true,
			User:         request.User,
			WorkingDir:   workspaceDirectory,
		},
		HostConfig: &container.HostConfig{
			AutoRemove: true,
			Mounts:     containerMounts(request),
		},
	})
	if err != nil {
		return mobyclient.ContainerCreateResult{}, fmt.Errorf("create container: %w", err)
	}

	return createdContainer, nil
}

func containerMounts(request RunRequest) []mount.Mount {
	mounts := make([]mount.Mount, 0, len(request.Mounts)+2)
	mounts = append(
		mounts,
		bindMount(request.WorkingDirectory, workspaceDirectory, false),
		bindMount(request.StateDirectory, homeDirectory, false),
	)
	for _, additionalMount := range request.Mounts {
		mounts = append(
			mounts,
			bindMount(
				additionalMount.Source,
				additionalMount.Target,
				additionalMount.ReadOnly,
			),
		)
	}

	return mounts
}

func bindMount(source string, target string, readOnly bool) mount.Mount {
	return mount.Mount{
		Type:     mount.TypeBind,
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}
}

func (c *Client) attachContainer(
	ctx context.Context,
	containerID string,
) (mobyclient.ContainerAttachResult, error) {
	attached, err := c.api.ContainerAttach(
		ctx,
		containerID,
		mobyclient.ContainerAttachOptions{
			Stdin:  true,
			Stdout: true,
			Stderr: true,
			Stream: true,
		},
	)
	if err != nil {
		return mobyclient.ContainerAttachResult{}, fmt.Errorf("attach to container: %w", err)
	}

	return attached, nil
}

func (c *Client) runContainer(
	ctx context.Context,
	containerID string,
	attached mobyclient.ContainerAttachResult,
	request RunRequest,
) error {
	wait := c.api.ContainerWait(ctx, containerID, mobyclient.ContainerWaitOptions{
		Condition: container.WaitConditionNextExit,
	})
	if _, err := c.api.ContainerStart(
		ctx,
		containerID,
		mobyclient.ContainerStartOptions{},
	); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	stopTerminalResize, err := c.forwardTerminalResize(ctx, containerID, request.Input)
	if err != nil {
		return err
	}
	defer stopTerminalResize()

	go copyInput(attached, request.Input)

	outputDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(outputWriter(request.Output), attached.Reader)
		outputDone <- err
	}()

	var waitError error

	select {
	case waitResult := <-wait.Result:
		if waitResult.Error != nil {
			return fmt.Errorf("wait for container: %s", waitResult.Error.Message)
		}
		if waitResult.StatusCode != 0 {
			waitError = fmt.Errorf("container exited with status %d", waitResult.StatusCode)
		}
	case err := <-wait.Error:
		return fmt.Errorf("wait for container: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := <-outputDone; err != nil {
		return fmt.Errorf("read container output: %w", err)
	}

	return waitError
}

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

func (c *Client) resizeTerminal(
	ctx context.Context,
	containerID string,
	terminal terminalInput,
) error {
	width, height, err := term.GetSize(terminal.Fd())
	if err != nil {
		return fmt.Errorf("get terminal size: %w", err)
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("get terminal size: invalid dimensions %dx%d", width, height)
	}

	_, err = c.api.ContainerResize(ctx, containerID, mobyclient.ContainerResizeOptions{
		Height: uint(height),
		Width:  uint(width),
	})
	if err != nil {
		return fmt.Errorf("resize container terminal: %w", err)
	}

	return nil
}

func copyInput(attached mobyclient.ContainerAttachResult, input io.Reader) {
	if input == nil {
		_ = attached.CloseWrite()

		return
	}

	_, _ = io.Copy(attached.Conn, input)
	_ = attached.CloseWrite()
}

func outputWriter(output io.Writer) io.Writer {
	if output == nil {
		return io.Discard
	}

	return output
}

type terminalInput interface {
	Fd() uintptr
}

func makeRawInput(input io.Reader) (func() error, error) {
	terminal, ok := input.(terminalInput)
	if !ok || !term.IsTerminal(terminal.Fd()) {
		return func() error {
			return nil
		}, nil
	}

	state, err := term.MakeRaw(terminal.Fd())
	if err != nil {
		return nil, err
	}

	return func() error {
		return term.Restore(terminal.Fd(), state)
	}, nil
}
