package networkaudit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/pixel365/agbx/internal/config"
)

const (
	stateHomeEnvironmentVariable = "XDG_STATE_HOME"
	caDirectoryName              = "network"
	caPEMFileName                = "mitmproxy-ca.pem"
	certificateFileName          = "mitmproxy-ca-cert.pem"
	runDirectoryTimeFormat       = "20060102T150405.000000000Z"
)

type Settings struct {
	CertificatePath string
	LogDirectory    string
	ProxyConfigPath string
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

	return Settings{
		CertificatePath: certificatePath,
		LogDirectory:    logDirectory,
		ProxyConfigPath: proxyConfigPath,
	}, nil
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
