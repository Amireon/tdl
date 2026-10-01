//go:build windows

package cmd

import "github.com/go-faster/errors"

func terminateWebLockHolders(string) error {
	return errors.New("automatic termination is not supported on Windows")
}
