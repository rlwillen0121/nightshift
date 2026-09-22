//go:build unix

package nightshift

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Polling in this platform helper keeps Run synchronous: no reader goroutine
// can remain blocked after quit, EOF, or a terminal error.
func waitForInput(file *os.File, timeout time.Duration) (bool, error) {
	if file == nil {
		return false, os.ErrInvalid
	}
	deadline := time.Now().Add(timeout)
	fd := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLIN}}
	for {
		remaining := time.Until(deadline)
		milliseconds := 0
		if remaining > 0 {
			milliseconds = int((remaining + time.Millisecond - 1) / time.Millisecond)
		}
		ready, err := unix.Poll(fd, milliseconds)
		if errors.Is(err, unix.EINTR) {
			if timeout <= 0 || time.Until(deadline) <= 0 {
				return false, nil
			}
			continue
		}
		if err != nil {
			return false, err
		}
		if ready == 0 {
			return false, nil
		}
		if fd[0].Revents&unix.POLLNVAL != 0 {
			return false, os.ErrInvalid
		}
		return fd[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) != 0, nil
	}
}
