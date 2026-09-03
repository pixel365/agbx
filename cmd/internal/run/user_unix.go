//go:build !windows

package run

import (
	"errors"
	"fmt"
	"os/user"
)

func currentUserIdentity() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	if currentUser.Uid == "" || currentUser.Gid == "" {
		return "", errors.New("current user UID and GID are required")
	}

	return currentUser.Uid + ":" + currentUser.Gid, nil
}
