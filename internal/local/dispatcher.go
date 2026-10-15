//go:build darwin || linux

package local

import (
	"errors"
	"os"
	"syscall"

	"github.com/stenhigh/prifly/internal/purity"
)

// DispatcherIdentity identifies the CLI itself; it need not lead a process group.
type DispatcherIdentity struct {
	PID, UID                  int
	Hostname, BootID, StartID string
}

func CurrentDispatcher() (DispatcherIdentity, error) {
	purity.Guard("dispatcher identity")
	p, err := readProcess(os.Getpid())
	if err != nil {
		return DispatcherIdentity{}, err
	}
	boot, err := processBootID()
	if err != nil {
		return DispatcherIdentity{}, err
	}
	host, err := os.Hostname()
	if err != nil {
		return DispatcherIdentity{}, err
	}
	return DispatcherIdentity{PID: p.PID, UID: p.UID, Hostname: host, BootID: boot, StartID: p.StartID}, nil
}

// DispatcherGone proves the former local process lifetime has ended. A new
// boot or a different native StartID cannot be that former lifetime; neither
// fact says anything about children or external effects, or authorizes signals.
func DispatcherGone(id DispatcherIdentity) bool {
	purity.Guard("dispatcher probe")
	if id.PID < 1 || id.UID < 0 || id.Hostname == "" || id.BootID == "" || id.StartID == "" {
		return false
	}
	host, err := os.Hostname()
	if err != nil || host != id.Hostname {
		return false
	}
	boot, err := processBootID()
	if err != nil {
		return false
	}
	if boot != id.BootID {
		return true
	}
	p, err := readProcess(id.PID)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return true
	}
	return err == nil && (p.StartID != id.StartID || p.UID == id.UID && p.Zombie)
}
