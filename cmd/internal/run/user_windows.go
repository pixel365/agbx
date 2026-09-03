//go:build windows

package run

func currentUserIdentity() (string, error) {
	return "", nil
}
