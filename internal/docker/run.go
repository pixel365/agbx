package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	mobyclient "github.com/moby/moby/client"
)

const (
	defaultWorkspaceDirectory = "/workspace"
	homeDirectory             = "/home/agbx"
	auditProxyHomeDirectory   = "/home/mitmproxy"
	auditCertificateTarget    = "/agbx/network-audit-ca.crt"
	auditProxyAlias           = "agbx-network-audit"
	auditProxyConfigDirectory = "/home/mitmproxy/.mitmproxy"
	auditProxyLogDirectory    = "/logs"
	auditProxyRedactionScript = "/agbx/redact.py"
	allCapabilities           = "ALL"
	noNewPrivileges           = "no-new-privileges=true"
	rootUser                  = "0:0"

	//nolint:nolintlint
	auditProxyImage = "mitmproxy/mitmproxy@sha256:00b77b5d8804c8ad18cb6caefbf9d5849e895e8986c5ce011f4ae30f4385962f"

	auditRuntimeEntrypoint = "/usr/local/bin/agbx-run"
	auditProxyReadyTimeout = 10 * time.Second
	auditProxyScriptOption = "-s"
	auditProxySetOption    = "--set"
)

type RunRequest struct {
	Input              io.Reader
	Output             io.Writer
	NetworkAudit       *NetworkAudit
	Image              string
	StateDirectory     string
	User               string
	WorkingDirectory   string
	WorkspaceDirectory string
	Mounts             []Mount
	Command            []string
	PullImage          bool
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type NetworkAudit struct {
	CertificatePath     string
	LogDirectory        string
	NetworkName         string
	ProxyConfigPath     string
	RedactionScriptPath string
}

func (c *Client) Run(ctx context.Context, request RunRequest) (runErr error) {
	if request.PullImage {
		if err := c.pullImage(ctx, request.Image); err != nil {
			return err
		}
	}
	if request.NetworkAudit != nil {
		defer removeAuditRedactionScript(request.NetworkAudit.RedactionScriptPath)

		stopAudit, err := c.startNetworkAudit(ctx, request.NetworkAudit, request.User)
		if err != nil {
			return err
		}
		defer stopAudit()
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

func (c *Client) startNetworkAudit(
	ctx context.Context,
	audit *NetworkAudit,
	user string,
) (func(), error) {
	networkName, err := newAuditNetworkName()
	if err != nil {
		return nil, err
	}
	audit.NetworkName = networkName
	if err := c.ensureImage(ctx, auditProxyImage); err != nil {
		return nil, err
	}
	if _, err := c.api.NetworkCreate(ctx, networkName, mobyclient.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels:   map[string]string{"app.agbx.network-audit": "true"},
	}); err != nil {
		return nil, fmt.Errorf("create network audit network: %w", err)
	}

	proxy, err := c.api.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config:     auditProxyConfig(audit, user),
		HostConfig: auditProxyHostConfig(audit),
	})
	if err != nil {
		c.removeNetworkAudit(audit, "")

		return nil, fmt.Errorf("create network audit proxy: %w", err)
	}
	if _, err := c.api.NetworkConnect(ctx, networkName, mobyclient.NetworkConnectOptions{
		Container: proxy.ID,
		EndpointConfig: &network.EndpointSettings{
			Aliases: []string{auditProxyAlias},
		},
	}); err != nil {
		c.removeNetworkAudit(audit, proxy.ID)

		return nil, fmt.Errorf("connect network audit proxy: %w", err)
	}
	if _, err := c.api.ContainerStart(
		ctx,
		proxy.ID,
		mobyclient.ContainerStartOptions{},
	); err != nil {
		c.removeNetworkAudit(audit, proxy.ID)

		return nil, fmt.Errorf("start network audit proxy: %w", err)
	}
	if err := c.waitForNetworkAuditProxy(ctx, proxy.ID); err != nil {
		c.removeNetworkAudit(audit, proxy.ID)

		return nil, err
	}

	return func() {
		c.removeNetworkAudit(audit, proxy.ID)
	}, nil
}

func auditProxyConfig(audit *NetworkAudit, user string) *container.Config {
	return &container.Config{
		Cmd: auditProxyCommand(audit),
		// The image entrypoint changes users through gosu, which needs privileges
		// unavailable under no-new-privileges. The configured confdir makes it unnecessary.
		Entrypoint: []string{""},
		Env:        []string{"HOME=" + auditProxyHomeDirectory},
		Image:      auditProxyImage,
		User:       user,
		Healthcheck: &container.HealthConfig{
			Interval:      time.Second,
			Retries:       3,
			StartInterval: 100 * time.Millisecond,
			StartPeriod:   5 * time.Second,
			Test: []string{
				"CMD",
				"python3",
				"-c",
				"import socket; socket.create_connection(('127.0.0.1', 8080), 1).close()",
			},
			Timeout: time.Second,
		},
	}
}

func auditProxyHostConfig(audit *NetworkAudit) *container.HostConfig {
	return &container.HostConfig{
		Mounts: auditProxyMounts(audit),
		// The proxy runs as the configured user and cannot gain new privileges.
		SecurityOpt: []string{noNewPrivileges},
	}
}

func auditProxyCommand(audit *NetworkAudit) []string {
	command := []string{
		"mitmdump",
		auditProxySetOption, "confdir=" + auditProxyConfigDirectory,
		auditProxySetOption, "hardump=" + auditProxyLogDirectory + "/flows.har",
		auditProxySetOption, "flow_detail=0",
		auditProxySetOption, "onboarding=false",
		auditProxySetOption, "save_stream_file=" + auditProxyLogDirectory + "/flows.mitm",
		auditProxySetOption, "store_streamed_bodies=true",
	}
	if audit.RedactionScriptPath != "" {
		command = append(command, auditProxyScriptOption, auditProxyRedactionScript)
	}

	return command
}

func auditProxyMounts(audit *NetworkAudit) []mount.Mount {
	mounts := []mount.Mount{
		bindMount(audit.LogDirectory, auditProxyLogDirectory, false),
		bindMount(audit.ProxyConfigPath, auditProxyConfigDirectory, false),
	}
	if audit.RedactionScriptPath != "" {
		mounts = append(
			mounts,
			bindMount(audit.RedactionScriptPath, auditProxyRedactionScript, true),
		)
	}

	return mounts
}

func (c *Client) waitForNetworkAuditProxy(ctx context.Context, proxyID string) error {
	readyContext, cancel := context.WithTimeout(ctx, auditProxyReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		inspection, err := c.api.ContainerInspect(
			readyContext,
			proxyID,
			mobyclient.ContainerInspectOptions{},
		)
		if err != nil {
			return fmt.Errorf("inspect network audit proxy: %w", err)
		}
		if inspection.Container.State == nil {
			return errors.New("inspect network audit proxy: container state is missing")
		}
		if !inspection.Container.State.Running {
			return c.auditProxyExitError(readyContext, proxyID, inspection.Container.State)
		}
		if inspection.Container.State.Health != nil {
			switch inspection.Container.State.Health.Status {
			case container.Healthy:
				return nil
			case container.NoHealthcheck, container.Starting:
				// Continue waiting for Docker to run the health check.
			case container.Unhealthy:
				return errors.New("network audit proxy is unhealthy")
			}
		}

		select {
		case <-readyContext.Done():
			return fmt.Errorf("wait for network audit proxy: %w", readyContext.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) auditProxyExitError(
	ctx context.Context,
	proxyID string,
	state *container.State,
) error {
	message := fmt.Sprintf("network audit proxy exited with status %d", state.ExitCode)
	if state.Error != "" {
		message += ": " + state.Error
	}
	logs, err := c.auditProxyLogs(ctx, proxyID)
	if err == nil && strings.TrimSpace(logs) != "" {
		message += ": " + strings.TrimSpace(logs)
	}

	return errors.New(message)
}

func (c *Client) ensureImage(ctx context.Context, image string) error {
	hasImage, err := c.HasImage(ctx, image)
	if err != nil {
		return fmt.Errorf("check network audit proxy image: %w", err)
	}
	if hasImage {
		return nil
	}
	if err := c.pullImage(ctx, image); err != nil {
		return fmt.Errorf("pull network audit proxy image: %w", err)
	}

	return nil
}

func (c *Client) removeNetworkAudit(audit *NetworkAudit, proxyID string) {
	ctx := context.Background()
	if proxyID != "" {
		_, _ = c.api.ContainerStop(ctx, proxyID, mobyclient.ContainerStopOptions{})
		c.writeAuditProxyLog(ctx, audit.LogDirectory, proxyID)
		_, _ = c.api.ContainerRemove(ctx, proxyID, mobyclient.ContainerRemoveOptions{Force: true})
	}
	_, _ = c.api.NetworkRemove(ctx, audit.NetworkName, mobyclient.NetworkRemoveOptions{})
}

func removeAuditRedactionScript(path string) {
	if path == "" {
		return
	}

	// #nosec G703 -- The script is created in the network audit run directory.
	_ = os.Remove(path)
}

func (c *Client) writeAuditProxyLog(ctx context.Context, directory string, proxyID string) {
	logs, err := c.auditProxyLogs(ctx, proxyID)
	if err != nil {
		return
	}

	filePath := filepath.Join(directory, "proxy.log")
	// #nosec G304 -- The log directory is explicitly selected in the audit configuration.
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()
	_, _ = file.WriteString(logs)
}

func (c *Client) auditProxyLogs(ctx context.Context, proxyID string) (string, error) {
	logs, err := c.api.ContainerLogs(ctx, proxyID, mobyclient.ContainerLogsOptions{
		ShowStderr: true,
		ShowStdout: true,
	})
	if err != nil {
		return "", err
	}
	defer func() {
		_ = logs.Close()
	}()

	var output bytes.Buffer
	if _, err := stdcopy.StdCopy(&output, &output, logs); err != nil {
		return "", err
	}

	return output.String(), nil
}

func newAuditNetworkName() (string, error) {
	identifier := make([]byte, 8)
	if _, err := rand.Read(identifier); err != nil {
		return "", fmt.Errorf("generate network audit identifier: %w", err)
	}

	return "agbx-audit-" + hex.EncodeToString(identifier), nil
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
			Cmd:          containerCommand(request),
			Env:          containerEnvironment(request),
			Image:        request.Image,
			OpenStdin:    true,
			Tty:          true,
			User:         containerUser(request),
			WorkingDir:   containerWorkspaceDirectory(request),
		},
		HostConfig:       containerHostConfig(request),
		NetworkingConfig: containerNetworkingConfig(request),
	})
	if err != nil {
		return mobyclient.ContainerCreateResult{}, fmt.Errorf("create container: %w", err)
	}

	return createdContainer, nil
}

func containerHostConfig(request RunRequest) *container.HostConfig {
	hostConfig := &container.HostConfig{
		AutoRemove:  true,
		Mounts:      containerMounts(request),
		NetworkMode: containerNetworkMode(request),
		SecurityOpt: []string{noNewPrivileges},
	}
	if request.NetworkAudit != nil {
		// The audit runtime starts as root to install the proxy CA, then lowers privileges.
		return hostConfig
	}
	hostConfig.CapDrop = []string{allCapabilities}

	return hostConfig
}

func containerCommand(request RunRequest) []string {
	if request.NetworkAudit == nil {
		return request.Command
	}

	command := make([]string, 0, len(request.Command)+2)
	command = append(command, auditRuntimeEntrypoint, request.User)

	return append(command, request.Command...)
}

func containerEnvironment(request RunRequest) []string {
	environment := []string{"HOME=" + homeDirectory}
	if request.NetworkAudit == nil {
		return environment
	}

	proxyURL := "http://" + auditProxyAlias + ":8080"
	return append(environment,
		"AGBX_PROXY_CA_CERTIFICATE="+auditCertificateTarget,
		"ALL_PROXY="+proxyURL,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"NODE_EXTRA_CA_CERTS="+auditCertificateTarget,
		"NODE_USE_ENV_PROXY=1",
		"NO_PROXY=localhost,127.0.0.1,::1",
		"all_proxy="+proxyURL,
		"http_proxy="+proxyURL,
		"https_proxy="+proxyURL,
		"no_proxy=localhost,127.0.0.1,::1",
	)
}

func containerUser(request RunRequest) string {
	if request.NetworkAudit != nil {
		// The audit runtime requires root to trust the proxy CA before dropping privileges.
		return rootUser
	}

	return request.User
}

func containerNetworkMode(request RunRequest) container.NetworkMode {
	if request.NetworkAudit == nil {
		return ""
	}

	return container.NetworkMode(request.NetworkAudit.NetworkName)
}

func containerNetworkingConfig(request RunRequest) *network.NetworkingConfig {
	if request.NetworkAudit == nil {
		return nil
	}

	return &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
		request.NetworkAudit.NetworkName: {},
	}}
}

func containerMounts(request RunRequest) []mount.Mount {
	mounts := make([]mount.Mount, 0, len(request.Mounts)+2)
	mounts = append(
		mounts,
		bindMount(request.WorkingDirectory, containerWorkspaceDirectory(request), false),
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
	if request.NetworkAudit != nil {
		mounts = append(
			mounts,
			bindMount(request.NetworkAudit.CertificatePath, auditCertificateTarget, true),
		)
	}

	return mounts
}

func containerWorkspaceDirectory(request RunRequest) string {
	if request.WorkspaceDirectory == "" {
		return defaultWorkspaceDirectory
	}

	return request.WorkspaceDirectory
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
