package networkaudit

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
)

func TestSetupCreatesCertificateAndLogDirectory(t *testing.T) {
	stateHome := t.TempDir()
	logDirectory := filepath.Join(t.TempDir(), "logs")
	t.Setenv(stateHomeEnvironmentVariable, stateHome)

	settings, err := Setup(config.AuditConfig{LogDirectory: logDirectory})

	require.NoError(t, err)
	assert.Equal(t, logDirectory, settings.LogDirectory)
	assert.Equal(t, filepath.Join(stateHome, "agbx", caDirectoryName), settings.ProxyConfigPath)
	assert.Equal(
		t,
		filepath.Join(settings.ProxyConfigPath, certificateFileName),
		settings.CertificatePath,
	)
	certificate, err := os.ReadFile(settings.CertificatePath)
	require.NoError(t, err)
	block, _ := pem.Decode(certificate)
	require.NotNil(t, block)
	parsedCertificate, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.True(t, parsedCertificate.IsCA)
	assert.DirExists(t, logDirectory)
}

func TestSetupReusesCertificate(t *testing.T) {
	stateHome := t.TempDir()
	logDirectory := t.TempDir()
	t.Setenv(stateHomeEnvironmentVariable, stateHome)

	first, err := Setup(config.AuditConfig{LogDirectory: logDirectory})
	require.NoError(t, err)
	firstCertificate, err := os.ReadFile(first.CertificatePath)
	require.NoError(t, err)
	second, err := Setup(config.AuditConfig{LogDirectory: logDirectory})
	require.NoError(t, err)
	secondCertificate, err := os.ReadFile(second.CertificatePath)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, firstCertificate, secondCertificate)
}
