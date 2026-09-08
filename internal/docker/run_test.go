package docker

import (
	"bytes"
	"context"
	"os"
	"path"
	"path/filepath"
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

func TestContainerMountsIncludesNetworkProxyCertificate(t *testing.T) {
	request := RunRequest{
		NetworkProxy: &NetworkProxy{CertificatePath: "/host/network-proxy-ca.crt"},
	}

	assert.Equal(t, []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   "/host/network-proxy-ca.crt",
			Target:   networkProxyCertificateTarget,
			ReadOnly: true,
		},
	}, containerMounts(request)[2:])
}

func TestContainerHostConfigDropsAllCapabilitiesWithoutNetworkProxy(t *testing.T) {
	hostConfig := containerHostConfig(RunRequest{})

	assert.Equal(t, []string{allCapabilities}, hostConfig.CapDrop)
	assert.Equal(t, []string{noNewPrivileges}, hostConfig.SecurityOpt)
}

func TestContainerHostConfigKeepsCapabilitiesForNetworkProxyBootstrap(t *testing.T) {
	hostConfig := containerHostConfig(RunRequest{NetworkProxy: &NetworkProxy{}})

	assert.Empty(t, hostConfig.CapDrop)
	assert.Equal(t, []string{noNewPrivileges}, hostConfig.SecurityOpt)
}

func TestContainerUserUsesRootForNetworkProxyBootstrap(t *testing.T) {
	assert.Equal(t, rootUser, containerUser(RunRequest{NetworkProxy: &NetworkProxy{}}))
}

func TestNetworkProxyEntrypointMapsMountOwner(t *testing.T) {
	entrypoint := networkProxyEntrypointCommand()

	assert.Equal(t, networkProxyEntrypoint, entrypoint[0])
	assert.Equal(t, networkProxyEntrypointName, entrypoint[3])
	assert.Contains(t, entrypoint[2], `usermod -o -u "$user_id" mitmproxy`)
	assert.Contains(t, entrypoint[2], `HOME="`+networkProxyHomeDirectory+`" gosu mitmproxy`)
}

func TestContainerCommandUsesNetworkProxyEntrypoint(t *testing.T) {
	request := RunRequest{
		Command:      []string{"provider", "--help"},
		NetworkProxy: &NetworkProxy{},
		User:         "1000:1000",
	}

	assert.Equal(
		t,
		[]string{networkProxyRuntimeEntrypoint, "1000:1000", "provider", "--help"},
		containerCommand(request),
	)
}

func TestContainerEnvironmentUsesNetworkProxy(t *testing.T) {
	request := RunRequest{NetworkProxy: &NetworkProxy{}}

	assert.Contains(
		t,
		containerEnvironment(request),
		"HTTPS_PROXY=http://"+networkProxyAlias+":8080",
	)
	assert.Contains(
		t,
		containerEnvironment(request),
		"AGBX_PROXY_CA_CERTIFICATE="+networkProxyCertificateTarget,
	)
	assert.Contains(t, containerEnvironment(request), "NODE_USE_ENV_PROXY=1")
	assert.Equal(t, rootUser, containerUser(request))
}

func TestNetworkProxyUsesRedactionScript(t *testing.T) {
	proxy := &NetworkProxy{
		LogDirectory:        "/host/logs",
		ProxyConfigPath:     "/host/proxy-config",
		RedactionScriptPath: "/host/redact.py",
	}

	command := networkProxyCommand(proxy)
	mounts := networkProxyMounts(proxy)

	assert.Equal(
		t,
		[]string{networkProxyScriptOption, networkProxyRedactionScript},
		command[len(command)-2:],
	)
	assert.Equal(t, mount.Mount{
		Type:     mount.TypeBind,
		Source:   proxy.RedactionScriptPath,
		Target:   networkProxyRedactionScript,
		ReadOnly: true,
	}, mounts[len(mounts)-1])
	assert.Contains(t, command, networkProxySetOption)
	assert.Contains(t, command, "flow_detail=0")
}

func TestNetworkProxyUsesPolicyScriptWithoutAuditLog(t *testing.T) {
	proxy := &NetworkProxy{
		PolicyScriptPath: "/host/proxy-config/policy-123.py",
		ProxyConfigPath:  "/host/proxy-config",
	}

	command := networkProxyCommand(proxy)
	mounts := networkProxyMounts(proxy)

	assert.Contains(
		t,
		command,
		path.Join(networkProxyConfigDirectory, filepath.Base(proxy.PolicyScriptPath)),
	)
	assert.NotContains(t, command, "hardump="+networkProxyLogDirectory+"/flows.har")
	assert.Len(t, mounts, 1)
	assert.Equal(t, mount.Mount{
		Type:   mount.TypeBind,
		Source: proxy.ProxyConfigPath,
		Target: networkProxyConfigDirectory,
	}, mounts[0])
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
