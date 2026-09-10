package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

func ID(configFile string) (string, error) {
	resolvedPath, err := filepath.EvalSymlinks(configFile)
	if err != nil {
		return "", fmt.Errorf("resolve config path %q: %w", configFile, err)
	}

	digest := sha256.Sum256([]byte(filepath.Clean(resolvedPath)))

	return hex.EncodeToString(digest[:]), nil
}
