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

func TestContainerMountsIncludesAuditCertificate(t *testing.T) {
	request := RunRequest{
		NetworkAudit: &NetworkAudit{CertificatePath: "/host/network-audit-ca.crt"},
	}

	assert.Equal(t, []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   "/host/network-audit-ca.crt",
			Target:   auditCertificateTarget,
			ReadOnly: true,
		},
	}, containerMounts(request)[2:])
}

func TestContainerHostConfigDropsAllCapabilitiesWithoutAudit(t *testing.T) {
	hostConfig := containerHostConfig(RunRequest{})

	assert.Equal(t, []string{allCapabilities}, hostConfig.CapDrop)
	assert.Equal(t, []string{noNewPrivileges}, hostConfig.SecurityOpt)
}

func TestContainerHostConfigKeepsCapabilitiesForAuditBootstrap(t *testing.T) {
	hostConfig := containerHostConfig(RunRequest{NetworkAudit: &NetworkAudit{}})

	assert.Empty(t, hostConfig.CapDrop)
	assert.Equal(t, []string{noNewPrivileges}, hostConfig.SecurityOpt)
}

func TestAuditProxyHostConfigPreventsNewPrivileges(t *testing.T) {
	hostConfig := auditProxyHostConfig(&NetworkAudit{})

	assert.Empty(t, hostConfig.CapDrop)
	assert.Equal(t, []string{noNewPrivileges}, hostConfig.SecurityOpt)
}

func TestContainerCommandUsesAuditEntrypoint(t *testing.T) {
	request := RunRequest{
		Command:      []string{"provider", "--help"},
		NetworkAudit: &NetworkAudit{},
		User:         "1000:1000",
	}

	assert.Equal(
		t,
		[]string{auditRuntimeEntrypoint, "1000:1000", "provider", "--help"},
		containerCommand(request),
	)
}

func TestContainerEnvironmentUsesAuditProxy(t *testing.T) {
	request := RunRequest{NetworkAudit: &NetworkAudit{}}

	assert.Contains(t, containerEnvironment(request), "HTTPS_PROXY=http://"+auditProxyAlias+":8080")
	assert.Contains(
		t,
		containerEnvironment(request),
		"AGBX_PROXY_CA_CERTIFICATE="+auditCertificateTarget,
	)
	assert.Contains(t, containerEnvironment(request), "NODE_USE_ENV_PROXY=1")
	assert.Empty(t, containerUser(request))
}

func TestAuditProxyUsesRedactionScript(t *testing.T) {
	audit := &NetworkAudit{
		LogDirectory:        "/host/logs",
		ProxyConfigPath:     "/host/proxy-config",
		RedactionScriptPath: "/host/redact.py",
	}

	command := auditProxyCommand(audit)
	mounts := auditProxyMounts(audit)

	assert.Equal(
		t,
		[]string{auditProxyScriptOption, auditProxyRedactionScript},
		command[len(command)-2:],
	)
	assert.Equal(t, mount.Mount{
		Type:     mount.TypeBind,
		Source:   audit.RedactionScriptPath,
		Target:   auditProxyRedactionScript,
		ReadOnly: true,
	}, mounts[len(mounts)-1])
	assert.Contains(t, command, auditProxySetOption)
	assert.Contains(t, command, "flow_detail=0")
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
