package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/moby/moby/api/types/container"
	mobyclient "github.com/moby/moby/client"
)

const workspaceDirectory = "/workspace"

type RunRequest struct {
	Input            io.Reader
	Output           io.Writer
	Image            string
	WorkingDirectory string
	Command          []string
}

func (c *Client) Run(ctx context.Context, request RunRequest) error {
	pullResponse, err := c.api.ImagePull(ctx, request.Image, mobyclient.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", request.Image, err)
	}
	if err := pullResponse.Wait(ctx); err != nil {
		return fmt.Errorf("pull image %q: %w", request.Image, err)
	}

	createdContainer, err := c.api.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config: &container.Config{
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          request.Command,
			Image:        request.Image,
			OpenStdin:    true,
			Tty:          true,
			WorkingDir:   workspaceDirectory,
		},
		HostConfig: &container.HostConfig{
			AutoRemove: true,
			Binds: []string{
				request.WorkingDirectory + ":" + workspaceDirectory,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer func() {
		_, _ = c.api.ContainerRemove(
			context.Background(),
			createdContainer.ID,
			mobyclient.ContainerRemoveOptions{Force: true},
		)
	}()

	attached, err := c.api.ContainerAttach(
		ctx,
		createdContainer.ID,
		mobyclient.ContainerAttachOptions{
			Stdin:  true,
			Stdout: true,
			Stderr: true,
			Stream: true,
		},
	)
	if err != nil {
		return fmt.Errorf("attach to container: %w", err)
	}
	defer attached.Close()

	wait := c.api.ContainerWait(ctx, createdContainer.ID, mobyclient.ContainerWaitOptions{
		Condition: container.WaitConditionNextExit,
	})
	if _, err := c.api.ContainerStart(
		ctx,
		createdContainer.ID,
		mobyclient.ContainerStartOptions{},
	); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

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
