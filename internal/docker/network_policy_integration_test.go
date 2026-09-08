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
	"github.com/pixel365/agbx/internal/networkaudit"
	"github.com/pixel365/agbx/internal/networkpolicy"
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

func TestNetworkPolicy(t *testing.T) {
	client := newNetworkPolicyTestClient(t)
	image := buildNetworkPolicyTestImage(t, client)
	audit := newNetworkPolicyTestAudit(t, networkpolicy.Policy{
		Default: networkpolicy.DefaultDeny,
		Allow:   []string{networkPolicyAllowedHost},
	})
	require.FileExists(t, audit.PolicyScriptPath)

	stopProxy, err := client.startNetworkAudit(t.Context(), audit)
	require.NoError(t, err)
	proxyStopped := false
	t.Cleanup(func() {
		if !proxyStopped {
			stopProxy()
		}
		removeNetworkAuditScripts(audit)
	})
	startNetworkPolicyTestServer(t, client, image, audit.NetworkName)

	allowedStatus := runNetworkPolicyRequest(
		t,
		client,
		image,
		audit,
		networkPolicyAllowedHost,
	)
	blockedStatus := runNetworkPolicyRequest(
		t,
		client,
		image,
		audit,
		networkPolicyBlockedHost,
	)

	assert.Equal(t, "200", allowedStatus)
	assert.Equal(t, "403", blockedStatus)

	stopProxy()
	proxyStopped = true
	removeNetworkAuditScripts(audit)
	assert.NoFileExists(t, audit.PolicyScriptPath)

	// #nosec G304 -- The file is created by the proxy in this test's audit run directory.
	flows, err := os.ReadFile(filepath.Join(audit.LogDirectory, "flows.har"))
	require.NoError(t, err)
	assert.Contains(t, string(flows), networkPolicyAllowedHost)
	assert.Contains(t, string(flows), networkPolicyBlockedHost)
	assert.Contains(t, string(flows), `"status": 403`)
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

func newNetworkPolicyTestAudit(t *testing.T, policy networkpolicy.Policy) *NetworkAudit {
	t.Helper()

	directory := networkPolicyTestDirectory(t)
	t.Setenv("XDG_STATE_HOME", directory)
	settings, err := networkaudit.Setup(&config.AuditConfig{
		LogDirectory: filepath.Join(directory, "audit"),
	}, &policy)
	require.NoError(t, err)

	return &NetworkAudit{
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
	audit *NetworkAudit,
	host string,
) string {
	t.Helper()

	workspace := networkPolicyTestDirectory(t)
	stateDirectory := networkPolicyTestDirectory(t)
	request := RunRequest{
		Command: []string{
			"sh",
			"-c",
			"env -u NO_PROXY -u no_proxy curl --silent --output /dev/null --write-out '%{http_code}' http://" +
				host + ":" + networkPolicyServerPort + " > " + defaultWorkspaceDirectory + "/" +
				networkPolicyStatusFileName,
		},
		Image:            image,
		Input:            bytes.NewReader(nil),
		NetworkAudit:     audit,
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
