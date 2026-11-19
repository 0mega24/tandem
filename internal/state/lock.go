package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrBusy means that another tdm process holds the lock of the space.
var ErrBusy = errors.New("another tdm command is changing this space; try again when it ends")

// Lock takes the lock of the space for this process, so two tdm processes never change one space
// at once: a failed one would undo what the other made. It does not wait. The lock file stays
// in the state directory (a space name cannot end in .lock); removing it while another process
// opens it would let both take a lock.
func (s Store) Lock(name string) (unlock func(), err error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("lock space %s: %w", name, err)
	}
	f, err := os.OpenFile(filepath.Join(s.dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: ValidateName keeps the name a plain file name
	if err != nil {
		return nil, fmt.Errorf("lock space %s: %w", name, err)
	}
	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close() // the lock error is the one to report
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("space %s: %w", name, ErrBusy)
		}
		return nil, fmt.Errorf("lock space %s: %w", name, err)
	}
	return func() { _ = f.Close() }, nil // closing the file releases the lock
}
