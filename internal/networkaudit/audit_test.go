package networkaudit

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
)

const (
	firstRunIdentifier  = "0000000000000001"
	secondRunIdentifier = "0000000000000002"
	thirdRunIdentifier  = "0000000000000003"
)

func TestSetupCreatesCertificateAndLogDirectory(t *testing.T) {
	stateHome := t.TempDir()
	logDirectory := filepath.Join(t.TempDir(), "logs")
	t.Setenv(stateHomeEnvironmentVariable, stateHome)

	settings, err := Setup(config.AuditConfig{LogDirectory: logDirectory})

	require.NoError(t, err)
	assert.NotEqual(t, logDirectory, settings.LogDirectory)
	assert.Equal(t, logDirectory, filepath.Dir(settings.LogDirectory))
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
	assert.DirExists(t, settings.LogDirectory)
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

	assert.NotEqual(t, first.LogDirectory, second.LogDirectory)
	assert.Equal(t, logDirectory, filepath.Dir(first.LogDirectory))
	assert.Equal(t, logDirectory, filepath.Dir(second.LogDirectory))
	assert.Equal(t, first.CertificatePath, second.CertificatePath)
	assert.Equal(t, first.ProxyConfigPath, second.ProxyConfigPath)
	assert.Equal(t, firstCertificate, secondCertificate)
}

func TestCleanupRunDirectoriesKeepsNewestRuns(t *testing.T) {
	logDirectory := t.TempDir()
	now := time.Date(2026, time.September, 7, 15, 0, 0, 0, time.UTC)
	oldestDirectory := createTestRunDirectory(
		t,
		logDirectory,
		now.Add(-2*time.Hour),
		firstRunIdentifier,
	)
	newerDirectory := createTestRunDirectory(
		t,
		logDirectory,
		now.Add(-time.Hour),
		secondRunIdentifier,
	)
	currentDirectory := createTestRunDirectory(t, logDirectory, now, thirdRunIdentifier)

	err := cleanupRunDirectories(
		config.AuditConfig{
			LogDirectory: logDirectory,
			Retention:    config.AuditRetention{MaxRuns: 2},
		},
		currentDirectory,
		now,
	)

	require.NoError(t, err)
	assertDirectoryRemoved(t, oldestDirectory)
	assert.DirExists(t, newerDirectory)
	assert.DirExists(t, currentDirectory)
}

func TestCleanupRunDirectoriesRemovesExpiredRunsOnly(t *testing.T) {
	logDirectory := t.TempDir()
	now := time.Date(2026, time.September, 7, 15, 0, 0, 0, time.UTC)
	expiredDirectory := createTestRunDirectory(
		t,
		logDirectory,
		now.Add(-2*time.Hour),
		firstRunIdentifier,
	)
	recentDirectory := createTestRunDirectory(
		t,
		logDirectory,
		now.Add(-30*time.Minute),
		secondRunIdentifier,
	)
	currentDirectory := createTestRunDirectory(t, logDirectory, now, thirdRunIdentifier)
	unmanagedDirectory := filepath.Join(logDirectory, "notes")
	require.NoError(t, os.Mkdir(unmanagedDirectory, 0o700))

	err := cleanupRunDirectories(
		config.AuditConfig{
			LogDirectory: logDirectory,
			Retention:    config.AuditRetention{MaxAge: "1h"},
		},
		currentDirectory,
		now,
	)

	require.NoError(t, err)
	assertDirectoryRemoved(t, expiredDirectory)
	assert.DirExists(t, recentDirectory)
	assert.DirExists(t, currentDirectory)
	assert.DirExists(t, unmanagedDirectory)
}

func createTestRunDirectory(
	t *testing.T,
	parentDirectory string,
	createdAt time.Time,
	identifier string,
) string {
	t.Helper()
	directory := filepath.Join(
		parentDirectory,
		createdAt.UTC().Format(runDirectoryTimeFormat)+"-"+identifier,
	)
	require.NoError(t, os.Mkdir(directory, 0o700))

	return directory
}

func assertDirectoryRemoved(t *testing.T, directory string) {
	t.Helper()
	_, err := os.Stat(directory)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
