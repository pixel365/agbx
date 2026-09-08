//go:build integration

package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	mobyclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/networkpolicy"
	"github.com/pixel365/agbx/internal/networkproxy"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	networkPolicyTestImageRepository = "agbx/network-policy-test"
	networkPolicyTestDirectoryPrefix = ".agbx-network-policy-test-"
	networkPolicyAllowedHost         = "allowed.example.test"
	networkPolicyBlockedHost         = "blocked.example.test"
	networkPolicyServerPort          = "80"
	networkPolicyStatusFileName      = "status"
)

func TestNetworkPolicyWithAudit(t *testing.T) {
	client := newNetworkPolicyTestClient(t)
	image := buildNetworkPolicyTestImage(t, client)
	auditDirectory := networkPolicyTestDirectory(t)
	proxy := newNetworkPolicyTestProxy(t, &config.AuditConfig{
		LogDirectory: filepath.Join(auditDirectory, "audit"),
	}, networkpolicy.Policy{
		Default: networkpolicy.DefaultDeny,
		Allow:   []string{networkPolicyAllowedHost},
	})
	require.FileExists(t, proxy.PolicyScriptPath)

	stopProxy, err := client.startNetworkProxy(t.Context(), proxy)
	require.NoError(t, err)
	proxyStopped := false
	t.Cleanup(func() {
		if !proxyStopped {
			stopProxy()
		}
		removeNetworkProxyScripts(proxy)
	})
	startNetworkPolicyTestServer(t, client, image, proxy.NetworkName)

	allowedStatus := runNetworkPolicyRequest(
		t,
		client,
		image,
		proxy,
		networkPolicyAllowedHost,
	)
	blockedStatus := runNetworkPolicyRequest(
		t,
		client,
		image,
		proxy,
		networkPolicyBlockedHost,
	)

	assert.Equal(t, "200", allowedStatus)
	assert.Equal(t, "403", blockedStatus)

	stopProxy()
	proxyStopped = true
	removeNetworkProxyScripts(proxy)
	assert.NoFileExists(t, proxy.PolicyScriptPath)

	// #nosec G304 -- The file is created by the proxy in this test's audit run directory.
	flows, err := os.ReadFile(filepath.Join(proxy.LogDirectory, "flows.har"))
	require.NoError(t, err)
	assert.Contains(t, string(flows), networkPolicyAllowedHost)
	assert.Contains(t, string(flows), networkPolicyBlockedHost)
	assert.Contains(t, string(flows), `"status": 403`)
}

func TestNetworkPolicyWithoutAudit(t *testing.T) {
	client := newNetworkPolicyTestClient(t)
	image := buildNetworkPolicyTestImage(t, client)
	proxy := newNetworkPolicyTestProxy(t, nil, networkpolicy.Policy{
		Default: networkpolicy.DefaultDeny,
		Allow:   []string{networkPolicyAllowedHost},
	})
	require.Empty(t, proxy.LogDirectory)

	stopProxy, err := client.startNetworkProxy(t.Context(), proxy)
	require.NoError(t, err)
	proxyStopped := false
	t.Cleanup(func() {
		if !proxyStopped {
			stopProxy()
		}
		removeNetworkProxyScripts(proxy)
	})
	startNetworkPolicyTestServer(t, client, image, proxy.NetworkName)

	allowedStatus := runNetworkPolicyRequest(
		t,
		client,
		image,
		proxy,
		networkPolicyAllowedHost,
	)
	blockedConnect := runNetworkPolicyHTTPSConnect(
		t,
		client,
		image,
		proxy,
		networkPolicyBlockedHost,
	)

	assert.Equal(t, "200", allowedStatus)
	assert.Contains(t, blockedConnect, "403")

	stopProxy()
	proxyStopped = true
	removeNetworkProxyScripts(proxy)
	assert.NoFileExists(t, proxy.PolicyScriptPath)
}

func newNetworkPolicyTestClient(t *testing.T) *Client {
	t.Helper()

	client, err := NewClient()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.Ping(ctx))

	return client
}

func buildNetworkPolicyTestImage(t *testing.T, client *Client) string {
	t.Helper()

	image := fmt.Sprintf("%s:%d", networkPolicyTestImageRepository, time.Now().UnixNano())
	recipe := provider.NewBuildRecipe(config.Image{Name: "alpine", Tag: "3.24"}, "")
	require.NoError(t, client.Build(t.Context(), BuildRequest{
		Dockerfile: recipe.Dockerfile,
		BuildArgs:  recipe.BuildArgs,
		Tag:        image,
	}))
	t.Cleanup(func() {
		_, err := client.api.ImageRemove(
			context.Background(),
			image,
			mobyclient.ImageRemoveOptions{Force: true},
		)
		require.NoError(t, err)
	})

	return image
}

func newNetworkPolicyTestProxy(
	t *testing.T,
	audit *config.AuditConfig,
	policy networkpolicy.Policy,
) *NetworkProxy {
	t.Helper()

	directory := networkPolicyTestDirectory(t)
	t.Setenv("XDG_STATE_HOME", directory)
	settings, err := networkproxy.Setup(audit, &policy)
	require.NoError(t, err)

	return &NetworkProxy{
		CertificatePath:     settings.CertificatePath,
		LogDirectory:        settings.LogDirectory,
		PolicyScriptPath:    settings.PolicyScriptPath,
		ProxyConfigPath:     settings.ProxyConfigPath,
		RedactionScriptPath: settings.RedactionScriptPath,
	}
}

func startNetworkPolicyTestServer(
	t *testing.T,
	client *Client,
	image string,
	networkName string,
) {
	t.Helper()

	server, err := client.api.ContainerCreate(t.Context(), mobyclient.ContainerCreateOptions{
		Config: &container.Config{
			Cmd:   []string{"python3", "-m", "http.server", networkPolicyServerPort},
			Image: image,
			Healthcheck: &container.HealthConfig{
				Interval: time.Second,
				Retries:  3,
				Test: []string{
					"CMD",
					"python3",
					"-c",
					"import socket; socket.create_connection(('127.0.0.1', 80), 1).close()",
				},
				Timeout: time.Second,
			},
		},
		HostConfig: &container.HostConfig{NetworkMode: container.NetworkMode(networkName)},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkName: {
					Aliases: []string{networkPolicyAllowedHost, networkPolicyBlockedHost},
				},
			},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := client.api.ContainerRemove(
			context.Background(),
			server.ID,
			mobyclient.ContainerRemoveOptions{Force: true},
		)
		require.NoError(t, err)
	})
	_, err = client.api.ContainerStart(t.Context(), server.ID, mobyclient.ContainerStartOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		inspection, err := client.api.ContainerInspect(
			t.Context(),
			server.ID,
			mobyclient.ContainerInspectOptions{},
		)

		return err == nil && inspection.Container.State != nil &&
			inspection.Container.State.Health != nil &&
			inspection.Container.State.Health.Status == container.Healthy
	}, 5*time.Second, 100*time.Millisecond)
}

func runNetworkPolicyRequest(
	t *testing.T,
	client *Client,
	image string,
	proxy *NetworkProxy,
	host string,
) string {
	command := "env -u NO_PROXY -u no_proxy curl --silent --output /dev/null --write-out '%{http_code}' http://" +
		host + ":" + networkPolicyServerPort

	return runNetworkPolicyCommand(t, client, image, proxy, command)
}

func runNetworkPolicyHTTPSConnect(
	t *testing.T,
	client *Client,
	image string,
	proxy *NetworkProxy,
	host string,
) string {
	// HTTPS and WSS establish a CONNECT tunnel before the proxy reaches the upstream host.
	command := "(env -u NO_PROXY -u no_proxy curl --silent --show-error --connect-timeout 2 --proxy http://" +
		networkProxyAlias + ":8080 https://" + host + ") || true"

	return runNetworkPolicyCommand(t, client, image, proxy, command)
}

func runNetworkPolicyCommand(
	t *testing.T,
	client *Client,
	image string,
	proxy *NetworkProxy,
	command string,
) string {
	t.Helper()

	workspace := networkPolicyTestDirectory(t)
	stateDirectory := networkPolicyTestDirectory(t)
	request := RunRequest{
		Command: []string{
			"sh",
			"-c",
			"{ " + command + "; } > " + defaultWorkspaceDirectory + "/" +
				networkPolicyStatusFileName + " 2>&1",
		},
		Image:            image,
		Input:            bytes.NewReader(nil),
		NetworkProxy:     proxy,
		StateDirectory:   stateDirectory,
		User:             networkPolicyTestUser(t),
		WorkingDirectory: workspace,
	}
	createdContainer, err := client.createContainer(t.Context(), request)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = client.api.ContainerRemove(
			context.Background(),
			createdContainer.ID,
			mobyclient.ContainerRemoveOptions{Force: true},
		)
	})
	attached, err := client.attachContainer(t.Context(), createdContainer.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		attached.Close()
	})
	require.NoError(t, client.runContainer(t.Context(), createdContainer.ID, attached, request))
	statusPath := filepath.Join(workspace, networkPolicyStatusFileName)
	// #nosec G304 -- The probe command creates the file in its test workspace.
	status, err := os.ReadFile(statusPath)
	require.NoError(t, err)

	return strings.TrimSpace(string(status))
}

func networkPolicyTestUser(t *testing.T) string {
	t.Helper()

	userID := os.Getuid()
	if userID < 1 {
		t.Skip("network policy integration test must not run as root")
	}

	return fmt.Sprintf("%d:%d", userID, os.Getgid())
}

func networkPolicyTestDirectory(t *testing.T) string {
	t.Helper()

	checkout, err := os.Getwd()
	require.NoError(t, err)
	directory, err := os.MkdirTemp(checkout, networkPolicyTestDirectoryPrefix)
	require.NoError(t, err)
	t.Cleanup(func() {
		// #nosec G703 -- The directory was created by this test below the checkout.
		require.NoError(t, os.RemoveAll(directory))
	})

	return directory
}
