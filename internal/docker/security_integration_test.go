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

	mobyclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/networkaudit"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	securityTestImageRepository = "agbx/security-test"
	securityTestRuntimeUser     = "1234:1234"
	securityTestDirectoryPrefix = ".agbx-security-test-"
	securityStateFileName       = "security-state"
	securityStateCommand        = `printf 'UID:\t'; id -u
printf 'GID:\t'; id -g
grep -E '^(Cap(Inh|Prm|Eff|Bnd|Amb)|NoNewPrivs):' /proc/self/status`
)

var securityTestBaseImages = []config.Image{
	{Name: "alpine", Tag: "3.24"},
	{Name: "debian", Tag: "bookworm-slim"},
}

func TestIntegrationAgentSecurityState(t *testing.T) {
	client := newSecurityTestClient(t)
	user := securityTestUser(t)
	for _, baseImage := range securityTestBaseImages {
		t.Run(baseImage.Name, func(t *testing.T) {
			image := buildSecurityTestImage(t, client, baseImage)

			t.Run("normal", func(t *testing.T) {
				output := runSecurityStateProbe(t, client, image, user, nil)
				requireUnprivilegedSecurityState(t, output, user)
			})
			t.Run("audit", func(t *testing.T) {
				output := runSecurityStateProbe(t, client, image, user, newSecurityTestAudit(t))
				requireUnprivilegedSecurityState(t, output, user)
			})
		})
	}
}

func newSecurityTestClient(t *testing.T) *Client {
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

func buildSecurityTestImage(t *testing.T, client *Client, baseImage config.Image) string {
	t.Helper()

	image := fmt.Sprintf(
		"%s:%s-%d",
		securityTestImageRepository,
		baseImage.Name,
		time.Now().UnixNano(),
	)
	recipe := provider.NewBuildRecipe(baseImage, "USER "+securityTestRuntimeUser)
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

func newSecurityTestAudit(t *testing.T) *NetworkAudit {
	t.Helper()

	directory := securityTestDirectory(t)
	t.Setenv("XDG_STATE_HOME", directory)
	settings, err := networkaudit.Setup(&config.AuditConfig{
		LogDirectory: filepath.Join(directory, "audit"),
	}, nil)
	require.NoError(t, err)
	return &NetworkAudit{
		CertificatePath:     settings.CertificatePath,
		LogDirectory:        settings.LogDirectory,
		ProxyConfigPath:     settings.ProxyConfigPath,
		RedactionScriptPath: settings.RedactionScriptPath,
	}
}

func runSecurityStateProbe(
	t *testing.T,
	client *Client,
	image string,
	user string,
	audit *NetworkAudit,
) string {
	t.Helper()

	workspace := securityTestDirectory(t)
	stateDirectory := securityTestDirectory(t)
	statePath := filepath.Join(workspace, securityStateFileName)

	require.NoError(t, client.Run(t.Context(), RunRequest{
		Command: []string{
			"sh",
			"-c",
			"{ " + securityStateCommand + "; } > " +
				defaultWorkspaceDirectory + "/" + securityStateFileName,
		},
		Image:            image,
		Input:            bytes.NewReader(nil),
		NetworkAudit:     audit,
		StateDirectory:   stateDirectory,
		User:             user,
		WorkingDirectory: workspace,
	}))
	// #nosec G304 -- The file is created by the probe below its test workspace.
	output, err := os.ReadFile(statePath)
	require.NoError(t, err)

	return string(output)
}

func securityTestUser(t *testing.T) string {
	t.Helper()

	userID := os.Getuid()
	if userID < 0 {
		t.Skip("host does not report a POSIX user ID")
	}
	require.NotZero(t, userID, "integration test must not run as root")

	return fmt.Sprintf("%d:%d", userID, os.Getgid())
}

func securityTestDirectory(t *testing.T) string {
	t.Helper()

	// Docker Desktop only exposes configured host directories to its Linux daemon.
	checkout, err := os.Getwd()
	require.NoError(t, err)
	directory, err := os.MkdirTemp(checkout, securityTestDirectoryPrefix)
	require.NoError(t, err)
	t.Cleanup(func() {
		// #nosec G703 -- The directory was created by this test below the checkout.
		require.NoError(t, os.RemoveAll(directory))
	})

	return directory
}

func parseSecurityState(t *testing.T, output string) map[string]string {
	t.Helper()

	state := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		state[fields[0]] = fields[1]
	}

	return state
}

func requireUnprivilegedSecurityState(t *testing.T, output string, user string) {
	t.Helper()

	userID, groupID, found := strings.Cut(user, ":")
	require.True(t, found)
	state := parseSecurityState(t, output)
	for name, expectedValue := range map[string]string{
		"UID:":        userID,
		"GID:":        groupID,
		"CapInh:":     "0000000000000000",
		"CapPrm:":     "0000000000000000",
		"CapEff:":     "0000000000000000",
		"CapBnd:":     "0000000000000000",
		"CapAmb:":     "0000000000000000",
		"NoNewPrivs:": "1",
	} {
		require.Equal(
			t,
			expectedValue,
			state[name],
			fmt.Sprintf("%s\nraw output: %q", name, output),
		)
	}
}
