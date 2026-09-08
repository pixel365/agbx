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
	"path"
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
	defaultWorkspaceDirectory     = "/workspace"
	homeDirectory                 = "/home/agbx"
	networkProxyHomeDirectory     = "/home/mitmproxy"
	networkProxyCertificateTarget = "/agbx/network-proxy-ca.crt"
	networkProxyAlias             = "agbx-network-proxy"
	networkProxyConfigDirectory   = "/home/mitmproxy/.mitmproxy"
	networkProxyLogDirectory      = "/logs"
	networkProxyRedactionScript   = "/agbx/redact.py"
	networkProxyEntrypoint        = "bash"
	networkProxyEntrypointName    = "agbx-network-proxy"
	allCapabilities               = "ALL"
	noNewPrivileges               = "no-new-privileges=true"
	rootUser                      = "0:0"

	//nolint:nolintlint
	networkProxyImage = "mitmproxy/mitmproxy@sha256:00b77b5d8804c8ad18cb6caefbf9d5849e895e8986c5ce011f4ae30f4385962f"

	networkProxyRuntimeEntrypoint = "/usr/local/bin/agbx-run"
	networkProxyReadyTimeout      = 10 * time.Second
	networkProxyScriptOption      = "-s"
	networkProxySetOption         = "--set"
)

type RunRequest struct {
	Input              io.Reader
	Output             io.Writer
	NetworkProxy       *NetworkProxy
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

type NetworkProxy struct {
	CertificatePath     string
	LogDirectory        string
	NetworkName         string
	PolicyScriptPath    string
	ProxyConfigPath     string
	RedactionScriptPath string
}

func (c *Client) Run(ctx context.Context, request RunRequest) (runErr error) {
	if request.PullImage {
		if err := c.pullImage(ctx, request.Image); err != nil {
			return err
		}
	}
	if request.NetworkProxy != nil {
		defer removeNetworkProxyScripts(request.NetworkProxy)

		stopProxy, err := c.startNetworkProxy(ctx, request.NetworkProxy)
		if err != nil {
			return err
		}
		defer stopProxy()
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

func (c *Client) startNetworkProxy(
	ctx context.Context,
	proxy *NetworkProxy,
) (func(), error) {
	networkName, err := newNetworkProxyName()
	if err != nil {
		return nil, err
	}
	proxy.NetworkName = networkName
	if err := c.ensureImage(ctx, networkProxyImage); err != nil {
		return nil, err
	}
	if _, err := c.api.NetworkCreate(ctx, networkName, mobyclient.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels:   map[string]string{"app.agbx.network-proxy": "true"},
	}); err != nil {
		return nil, fmt.Errorf("create network proxy network: %w", err)
	}

	proxyContainer, err := c.api.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config:     networkProxyConfig(proxy),
		HostConfig: networkProxyHostConfig(proxy),
	})
	if err != nil {
		c.removeNetworkProxy(proxy, "")

		return nil, fmt.Errorf("create network proxy: %w", err)
	}
	if _, err := c.api.NetworkConnect(ctx, networkName, mobyclient.NetworkConnectOptions{
		Container: proxyContainer.ID,
		EndpointConfig: &network.EndpointSettings{
			Aliases: []string{networkProxyAlias},
		},
	}); err != nil {
		c.removeNetworkProxy(proxy, proxyContainer.ID)

		return nil, fmt.Errorf("connect network proxy: %w", err)
	}
	if _, err := c.api.ContainerStart(
		ctx,
		proxyContainer.ID,
		mobyclient.ContainerStartOptions{},
	); err != nil {
		c.removeNetworkProxy(proxy, proxyContainer.ID)

		return nil, fmt.Errorf("start network proxy: %w", err)
	}
	if err := c.waitForNetworkProxy(ctx, proxyContainer.ID); err != nil {
		c.removeNetworkProxy(proxy, proxyContainer.ID)

		return nil, err
	}

	return func() {
		c.removeNetworkProxy(proxy, proxyContainer.ID)
	}, nil
}

func networkProxyConfig(proxy *NetworkProxy) *container.Config {
	return &container.Config{
		Cmd:        networkProxyCommand(proxy),
		Entrypoint: networkProxyEntrypointCommand(),
		Image:      networkProxyImage,
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

func networkProxyEntrypointCommand() []string {
	script := `set -eu
user_id="$(stat -c '%u' "` + networkProxyConfigDirectory + `")"
usermod -o -u "$user_id" mitmproxy >/dev/null
exec env HOME="` + networkProxyHomeDirectory + `" gosu mitmproxy "$@"`

	return []string{networkProxyEntrypoint, "-c", script, networkProxyEntrypointName}
}

func networkProxyHostConfig(proxy *NetworkProxy) *container.HostConfig {
	// Proxy state is owned by the host user, while the upstream image may not have its group.
	return &container.HostConfig{
		Mounts: networkProxyMounts(proxy),
	}
}

func networkProxyCommand(proxy *NetworkProxy) []string {
	command := []string{
		"mitmdump",
		networkProxySetOption, "confdir=" + networkProxyConfigDirectory,
		networkProxySetOption, "flow_detail=0",
		networkProxySetOption, "onboarding=false",
	}
	if proxy.LogDirectory != "" {
		command = append(command,
			networkProxySetOption, "hardump="+networkProxyLogDirectory+"/flows.har",
			networkProxySetOption, "save_stream_file="+networkProxyLogDirectory+"/flows.mitm",
			networkProxySetOption, "store_streamed_bodies=true",
		)
	}
	if proxy.PolicyScriptPath != "" {
		command = append(
			command,
			networkProxyScriptOption,
			path.Join(networkProxyConfigDirectory, filepath.Base(proxy.PolicyScriptPath)),
		)
	}
	if proxy.RedactionScriptPath != "" {
		command = append(command, networkProxyScriptOption, networkProxyRedactionScript)
	}

	return command
}

func networkProxyMounts(proxy *NetworkProxy) []mount.Mount {
	mounts := []mount.Mount{bindMount(proxy.ProxyConfigPath, networkProxyConfigDirectory, false)}
	if proxy.LogDirectory != "" {
		mounts = append(mounts, bindMount(proxy.LogDirectory, networkProxyLogDirectory, false))
	}
	if proxy.RedactionScriptPath != "" {
		mounts = append(
			mounts,
			bindMount(proxy.RedactionScriptPath, networkProxyRedactionScript, true),
		)
	}

	return mounts
}

func (c *Client) waitForNetworkProxy(ctx context.Context, proxyID string) error {
	readyContext, cancel := context.WithTimeout(ctx, networkProxyReadyTimeout)
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
			return fmt.Errorf("inspect network proxy: %w", err)
		}
		if inspection.Container.State == nil {
			return errors.New("inspect network proxy: container state is missing")
		}
		if !inspection.Container.State.Running {
			return c.networkProxyExitError(readyContext, proxyID, inspection.Container.State)
		}
		if inspection.Container.State.Health != nil {
			switch inspection.Container.State.Health.Status {
			case container.Healthy:
				return nil
			case container.NoHealthcheck, container.Starting:
				// Continue waiting for Docker to run the health check.
			case container.Unhealthy:
				return errors.New("network proxy is unhealthy")
			}
		}

		select {
		case <-readyContext.Done():
			return fmt.Errorf("wait for network proxy: %w", readyContext.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) networkProxyExitError(
	ctx context.Context,
	proxyID string,
	state *container.State,
) error {
	message := fmt.Sprintf("network proxy exited with status %d", state.ExitCode)
	if state.Error != "" {
		message += ": " + state.Error
	}
	logs, err := c.networkProxyLogs(ctx, proxyID)
	if err == nil && strings.TrimSpace(logs) != "" {
		message += ": " + strings.TrimSpace(logs)
	}

	return errors.New(message)
}

func (c *Client) ensureImage(ctx context.Context, image string) error {
	hasImage, err := c.HasImage(ctx, image)
	if err != nil {
		return fmt.Errorf("check network proxy image: %w", err)
	}
	if hasImage {
		return nil
	}
	if err := c.pullImage(ctx, image); err != nil {
		return fmt.Errorf("pull network proxy image: %w", err)
	}

	return nil
}

func (c *Client) removeNetworkProxy(proxy *NetworkProxy, proxyID string) {
	ctx := context.Background()
	if proxyID != "" {
		_, _ = c.api.ContainerStop(ctx, proxyID, mobyclient.ContainerStopOptions{})
		if proxy.LogDirectory != "" {
			c.writeNetworkProxyLog(ctx, proxy.LogDirectory, proxyID)
		}
		_, _ = c.api.ContainerRemove(ctx, proxyID, mobyclient.ContainerRemoveOptions{Force: true})
	}
	_, _ = c.api.NetworkRemove(ctx, proxy.NetworkName, mobyclient.NetworkRemoveOptions{})
}

func removeNetworkProxyScripts(proxy *NetworkProxy) {
	for _, path := range []string{proxy.PolicyScriptPath, proxy.RedactionScriptPath} {
		if path == "" {
			continue
		}

		// #nosec G703 -- Each script is created by agbx in its private local state.
		_ = os.Remove(path)
	}
}

func (c *Client) writeNetworkProxyLog(ctx context.Context, directory string, proxyID string) {
	logs, err := c.networkProxyLogs(ctx, proxyID)
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

func (c *Client) networkProxyLogs(ctx context.Context, proxyID string) (string, error) {
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

func newNetworkProxyName() (string, error) {
	identifier := make([]byte, 8)
	if _, err := rand.Read(identifier); err != nil {
		return "", fmt.Errorf("generate network proxy identifier: %w", err)
	}

	return "agbx-proxy-" + hex.EncodeToString(identifier), nil
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
	if request.NetworkProxy != nil {
		// The proxy runtime starts as root to install the proxy CA, then lowers privileges.
		return hostConfig
	}
	hostConfig.CapDrop = []string{allCapabilities}

	return hostConfig
}

func containerCommand(request RunRequest) []string {
	if request.NetworkProxy == nil {
		return request.Command
	}

	command := make([]string, 0, len(request.Command)+2)
	command = append(command, networkProxyRuntimeEntrypoint, request.User)

	return append(command, request.Command...)
}

func containerEnvironment(request RunRequest) []string {
	environment := []string{"HOME=" + homeDirectory}
	if request.NetworkProxy == nil {
		return environment
	}

	proxyURL := "http://" + networkProxyAlias + ":8080"
	return append(environment,
		"AGBX_PROXY_CA_CERTIFICATE="+networkProxyCertificateTarget,
		"ALL_PROXY="+proxyURL,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"NODE_EXTRA_CA_CERTS="+networkProxyCertificateTarget,
		"NODE_USE_ENV_PROXY=1",
		"NO_PROXY=localhost,127.0.0.1,::1",
		"all_proxy="+proxyURL,
		"http_proxy="+proxyURL,
		"https_proxy="+proxyURL,
		"no_proxy=localhost,127.0.0.1,::1",
	)
}

func containerUser(request RunRequest) string {
	if request.NetworkProxy != nil {
		// The proxy runtime requires root to trust the proxy CA before dropping privileges.
		return rootUser
	}

	return request.User
}

func containerNetworkMode(request RunRequest) container.NetworkMode {
	if request.NetworkProxy == nil {
		return ""
	}

	return container.NetworkMode(request.NetworkProxy.NetworkName)
}

func containerNetworkingConfig(request RunRequest) *network.NetworkingConfig {
	if request.NetworkProxy == nil {
		return nil
	}

	return &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
		request.NetworkProxy.NetworkName: {},
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
	if request.NetworkProxy != nil {
		mounts = append(
			mounts,
			bindMount(request.NetworkProxy.CertificatePath, networkProxyCertificateTarget, true),
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
