package networkaudit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pixel365/agbx/internal/config"
)

const (
	stateHomeEnvironmentVariable = "XDG_STATE_HOME"
	caDirectoryName              = "network"
	caPEMFileName                = "mitmproxy-ca.pem"
	certificateFileName          = "mitmproxy-ca-cert.pem"
	redactionScriptFileName      = "redact.py"
	runDirectoryTimeFormat       = "20060102T150405.000000000Z"
	redactedHeaderValue          = "[REDACTED]"
	redactionScriptTemplate      = `from mitmproxy import http

REDACTED_HEADERS = frozenset(%s)
REDACTED_QUERY_PARAMETERS = frozenset(%s)
REDACTED_VALUE = %q


def redact_headers(headers) -> None:
    for name in list(headers.keys()):
        if name.lower() in REDACTED_HEADERS:
            headers[name] = REDACTED_VALUE


def redact_query_parameters(query) -> None:
    for name in list(query.keys()):
        if name in REDACTED_QUERY_PARAMETERS:
            query[name] = REDACTED_VALUE


def redact_request(request) -> None:
    redact_headers(request.headers)
    redact_query_parameters(request.query)


def response(flow: http.HTTPFlow) -> None:
    redact_request(flow.request)


def error(flow: http.HTTPFlow) -> None:
    redact_request(flow.request)
`
)

type Settings struct {
	CertificatePath     string
	LogDirectory        string
	ProxyConfigPath     string
	RedactionScriptPath string
}

func Setup(configuration config.AuditConfig) (Settings, error) {
	if err := ensureDirectory(configuration.LogDirectory); err != nil {
		return Settings{}, fmt.Errorf(
			"create network audit log directory %q: %w",
			configuration.LogDirectory,
			err,
		)
	}
	proxyConfigPath, err := proxyConfigDirectory()
	if err != nil {
		return Settings{}, err
	}
	certificatePath, err := ensureCertificate(proxyConfigPath)
	if err != nil {
		return Settings{}, err
	}
	logDirectory, err := createRunDirectory(configuration.LogDirectory)
	if err != nil {
		return Settings{}, fmt.Errorf(
			"create network audit run directory in %q: %w",
			configuration.LogDirectory, err,
		)
	}
	if hasRetention(configuration.Retention) {
		if err := cleanupRunDirectories(configuration, logDirectory, time.Now().UTC()); err != nil {
			return Settings{}, err
		}
	}
	redactionScriptPath, err := createRedactionScript(
		logDirectory,
		configuration.Redact.Headers,
		configuration.Redact.QueryParameters,
	)
	if err != nil {
		return Settings{}, err
	}

	return Settings{
		CertificatePath:     certificatePath,
		LogDirectory:        logDirectory,
		ProxyConfigPath:     proxyConfigPath,
		RedactionScriptPath: redactionScriptPath,
	}, nil
}

func createRedactionScript(
	directory string,
	headerNames []string,
	queryParameterNames []string,
) (string, error) {
	if len(headerNames) == 0 && len(queryParameterNames) == 0 {
		return "", nil
	}
	encodedHeaderNames, err := json.Marshal(normalizedHeaderNames(headerNames))
	if err != nil {
		return "", fmt.Errorf("encode network audit redacted header names: %w", err)
	}
	encodedQueryParameterNames, err := json.Marshal(
		normalizedQueryParameterNames(queryParameterNames),
	)
	if err != nil {
		return "", fmt.Errorf("encode network audit redacted query parameter names: %w", err)
	}
	script := fmt.Sprintf(
		redactionScriptTemplate,
		encodedHeaderNames,
		encodedQueryParameterNames,
		redactedHeaderValue,
	)
	path := filepath.Join(directory, redactionScriptFileName)
	if err := writeFile(path, []byte(script)); err != nil {
		return "", fmt.Errorf("write network audit redaction script: %w", err)
	}

	return path, nil
}

func normalizedHeaderNames(headerNames []string) []string {
	normalized := make([]string, 0, len(headerNames))
	for _, name := range headerNames {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(name)))
	}

	return normalized
}

func normalizedQueryParameterNames(parameterNames []string) []string {
	normalized := make([]string, 0, len(parameterNames))
	for _, name := range parameterNames {
		normalized = append(normalized, strings.TrimSpace(name))
	}

	return normalized
}

func hasRetention(retention config.AuditRetention) bool {
	return retention.MaxRuns > 0 || retention.MaxAge != ""
}

type runDirectory struct {
	createdAt time.Time
	path      string
}

func cleanupRunDirectories(
	configuration config.AuditConfig,
	currentDirectory string,
	now time.Time,
) error {
	maxAge, err := retentionMaxAge(configuration.Retention.MaxAge)
	if err != nil {
		return err
	}
	runDirectories, err := auditRunDirectories(configuration.LogDirectory)
	if err != nil {
		return fmt.Errorf("list network audit run directories: %w", err)
	}
	runDirectories, err = removeExpiredRuns(runDirectories, currentDirectory, now, maxAge)
	if err != nil {
		return err
	}
	if err := removeExcessRuns(
		runDirectories,
		currentDirectory,
		configuration.Retention.MaxRuns,
	); err != nil {
		return err
	}

	return nil
}

func retentionMaxAge(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}

	maxAge, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse network audit max age %q: %w", value, err)
	}
	if maxAge < 0 {
		return 0, errors.New("network audit max age must not be negative")
	}

	return maxAge, nil
}

func auditRunDirectories(parentDirectory string) ([]runDirectory, error) {
	entries, err := os.ReadDir(parentDirectory)
	if err != nil {
		return nil, err
	}

	runDirectories := make([]runDirectory, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		createdAt, ok := parseRunDirectoryName(entry.Name())
		if !ok {
			continue
		}
		runDirectories = append(runDirectories, runDirectory{
			createdAt: createdAt,
			path:      filepath.Join(parentDirectory, entry.Name()),
		})
	}
	sort.Slice(runDirectories, func(first int, second int) bool {
		return runDirectories[first].createdAt.Before(runDirectories[second].createdAt)
	})

	return runDirectories, nil
}

func parseRunDirectoryName(name string) (time.Time, bool) {
	parts := strings.Split(name, "-")
	if len(parts) != 2 || len(parts[1]) != 16 {
		return time.Time{}, false
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return time.Time{}, false
	}

	createdAt, err := time.Parse(runDirectoryTimeFormat, parts[0])
	if err != nil {
		return time.Time{}, false
	}

	return createdAt, true
}

func removeExpiredRuns(
	runDirectories []runDirectory,
	currentDirectory string,
	now time.Time,
	maxAge time.Duration,
) ([]runDirectory, error) {
	if maxAge == 0 {
		return runDirectories, nil
	}

	cutoff := now.Add(-maxAge)
	remaining := make([]runDirectory, 0, len(runDirectories))
	for _, directory := range runDirectories {
		if directory.path == currentDirectory || !directory.createdAt.Before(cutoff) {
			remaining = append(remaining, directory)

			continue
		}
		if err := removeRunDirectory(directory.path); err != nil {
			return nil, err
		}
	}

	return remaining, nil
}

func removeExcessRuns(runDirectories []runDirectory, currentDirectory string, maxRuns int) error {
	if maxRuns == 0 || len(runDirectories) <= maxRuns {
		return nil
	}

	toRemove := len(runDirectories) - maxRuns
	for _, directory := range runDirectories {
		if toRemove == 0 {
			break
		}
		if directory.path == currentDirectory {
			continue
		}
		if err := removeRunDirectory(directory.path); err != nil {
			return err
		}
		toRemove--
	}

	return nil
}

func removeRunDirectory(directory string) error {
	// #nosec G703 -- The path is a validated direct child using agbx's run-directory format.
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("remove network audit run directory %q: %w", directory, err)
	}

	return nil
}

func createRunDirectory(parentDirectory string) (string, error) {
	identifier := make([]byte, 8)
	if _, err := rand.Read(identifier); err != nil {
		return "", fmt.Errorf("generate network audit run identifier: %w", err)
	}
	directory := filepath.Join(
		parentDirectory,
		time.Now().UTC().Format(runDirectoryTimeFormat)+"-"+hex.EncodeToString(identifier),
	)
	// #nosec G302,G703 -- The parent directory is explicitly selected in the audit configuration.
	if err := os.Mkdir(directory, 0o700); err != nil {
		return "", err
	}

	return directory, nil
}

func proxyConfigDirectory() (string, error) {
	stateHome := os.Getenv(stateHomeEnvironmentVariable)
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home directory for network audit: %w", err)
		}

		stateHome = filepath.Join(home, ".local", "state")
	}
	directory := filepath.Join(stateHome, "agbx", caDirectoryName)
	if err := ensureDirectory(directory); err != nil {
		return "", fmt.Errorf("create network audit state directory %q: %w", directory, err)
	}

	return directory, nil
}

func ensureDirectory(directory string) error {
	directory = filepath.Clean(directory)

	// #nosec G703 -- The audit directories are explicitly selected by the user or derived from local state.
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	// #nosec G703 -- The audit directories are explicitly selected by the user or derived from local state.
	info, err := os.Stat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	// #nosec G302 -- Audit directories require execute permission for the owning user.
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}

	return nil
}

func ensureCertificate(directory string) (string, error) {
	certificatePath := filepath.Join(directory, certificateFileName)
	privateCertificatePath := filepath.Join(directory, caPEMFileName)
	certificateExists, err := regularFileExists(certificatePath)
	if err != nil {
		return "", fmt.Errorf("inspect network audit certificate %q: %w", certificatePath, err)
	}
	privateCertificateExists, err := regularFileExists(privateCertificatePath)
	if err != nil {
		return "", fmt.Errorf(
			"inspect network audit certificate authority %q: %w",
			privateCertificatePath,
			err,
		)
	}
	if certificateExists && privateCertificateExists {
		return certificatePath, nil
	}
	if certificateExists || privateCertificateExists {
		return "", errors.New("network audit certificate files are incomplete")
	}

	privateCertificate, certificate, err := newCertificateAuthority()
	if err != nil {
		return "", err
	}
	if err := writeFile(privateCertificatePath, privateCertificate); err != nil {
		return "", fmt.Errorf(
			"write network audit certificate authority %q: %w",
			privateCertificatePath,
			err,
		)
	}
	if err := writeFile(certificatePath, certificate); err != nil {
		return "", fmt.Errorf("write network audit certificate %q: %w", certificatePath, err)
	}

	return certificatePath, nil
}

func regularFileExists(filePath string) (bool, error) {
	// #nosec G703 -- The audit paths are derived from the user's local state directory.
	info, err := os.Stat(filePath)
	if err == nil {
		if !info.Mode().IsRegular() {
			return false, errors.New("path is not a regular file")
		}

		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, err
}

func newCertificateAuthority() ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate network audit private key: %w", err)
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate network audit certificate serial number: %w", err)
	}
	certificateTemplate := x509.Certificate{
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		NotAfter:              time.Now().AddDate(10, 0, 0),
		NotBefore:             time.Now().Add(-time.Hour),
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: "agbx network audit"},
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader,
		&certificateTemplate,
		&certificateTemplate,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create network audit certificate: %w", err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("encode network audit private key: %w", err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	privateCertificate := append(
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}),
		certificate...,
	)

	return privateCertificate, certificate, nil
}

func writeFile(filePath string, contents []byte) error {
	// #nosec G304 -- The certificate files are created in the local audit state directory.
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		if removeErr := os.Remove(filePath); removeErr != nil {
			return errors.Join(err, removeErr)
		}

		return err
	}

	return file.Close()
}
