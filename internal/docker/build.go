package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/moby/moby/api/types/jsonstream"
	mobyclient "github.com/moby/moby/client"
)

const dockerfileName = "Dockerfile"

type BuildRequest struct {
	Dockerfile string
	BuildArgs  map[string]string
	Output     io.Writer
	Tag        string
}

func (c *Client) Build(ctx context.Context, request BuildRequest) error {
	contextArchive, err := buildContext(request.Dockerfile)
	if err != nil {
		return fmt.Errorf("create build context: %w", err)
	}

	build, err := c.api.ImageBuild(ctx, contextArchive, mobyclient.ImageBuildOptions{
		BuildArgs:  imageBuildArgs(request.BuildArgs),
		Dockerfile: dockerfileName,
		Remove:     true,
		Tags:       []string{request.Tag},
	})
	if err != nil {
		return fmt.Errorf("build image %q: %w", request.Tag, err)
	}
	defer func() {
		_ = build.Body.Close()
	}()

	if err := displayBuildOutput(build.Body, buildOutput(request.Output)); err != nil {
		return fmt.Errorf("read image build output: %w", err)
	}

	return nil
}

func imageBuildArgs(arguments map[string]string) map[string]*string {
	buildArgs := make(map[string]*string, len(arguments))
	for name, value := range arguments {
		buildArgs[name] = &value
	}

	return buildArgs
}

func buildContext(dockerfile string) (*bytes.Reader, error) {
	var contents bytes.Buffer
	archive := tar.NewWriter(&contents)
	if err := archive.WriteHeader(&tar.Header{
		Mode: 0o600,
		Name: dockerfileName,
		Size: int64(len(dockerfile)),
	}); err != nil {
		return nil, fmt.Errorf("write Dockerfile header: %w", err)
	}
	if _, err := archive.Write([]byte(dockerfile)); err != nil {
		return nil, fmt.Errorf("write Dockerfile: %w", err)
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("close build context: %w", err)
	}

	return bytes.NewReader(contents.Bytes()), nil
}

func buildOutput(output io.Writer) io.Writer {
	if output == nil {
		return io.Discard
	}

	return output
}

func displayBuildOutput(input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	for {
		var message jsonstream.Message
		if err := decoder.Decode(&message); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}
		if message.Error != nil {
			return message.Error
		}
		if _, err := io.WriteString(output, message.Stream); err != nil {
			return err
		}
	}
}
