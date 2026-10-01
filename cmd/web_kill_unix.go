//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-faster/errors"
	"go.uber.org/multierr"
)

// terminateWebLockHolders kills stale `tdl web` processes that hold the bolt
// database file open, so a new web instance can take over (auto-restart).
// Only processes whose argv contains the "web" subcommand are touched; other
// tdl commands (e.g. a running download) are left alone.
func terminateWebLockHolders(dbPath string) error {
	out, err := exec.Command("lsof", "-t", "--", dbPath).Output()
	if err != nil {
		return errors.Wrap(err, "lsof")
	}

	var errs []error
	killed := false
	for _, s := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(s)
		if err != nil || pid == os.Getpid() {
			continue
		}
		if !isWebProcess(pid) {
			continue
		}
		if err := terminateProcess(pid); err != nil {
			errs = append(errs, errors.Wrapf(err, "kill %d", pid))
			continue
		}
		killed = true
	}
	if !killed && len(errs) == 0 {
		return errors.New("no stale tdl web process holds the lock")
	}
	return multierr.Combine(errs...)
}

func isWebProcess(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || !strings.Contains(fields[0], "tdl") {
		return false
	}
	for _, f := range fields[1:] {
		if f == "web" {
			return true
		}
	}
	return false
}

func terminateProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err = p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	if waitProcessExit(pid, 3*time.Second) {
		return nil
	}
	if err = p.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	if waitProcessExit(pid, 2*time.Second) {
		return nil
	}
	return fmt.Errorf("process %d did not exit", pid)
}

// waitProcessExit polls until the process is gone or d elapses. The process
// is not our child, so we cannot wait(2) on it and probe with signal 0.
func waitProcessExit(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
