//go:build unix

package nightshift

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestPTYStartupErrorPreservesTerminal(t *testing.T) {
	pty, tty := openPTY(t)
	defer pty.Close()
	defer tty.Close()
	before := terminalState(t, tty)
	stderr := tempFile(t)
	err := Run([]string{"--seed", "ZZ-BAD-SEED-991"}, tty, tty, stderr)
	after := terminalState(t, tty)
	if ExitCode(err) != 2 || bytes.Contains([]byte(err.Error()), []byte("ZZ-BAD-SEED-991")) {
		t.Fatal(err)
	}
	if *before != *after {
		t.Fatal("startup error changed terminal state")
	}
}

func TestPTYCtrlCIsSoleUserExitAndRestoresTerminal(t *testing.T) {
	pty, tty := openPTY(t)
	defer pty.Close()
	defer tty.Close()
	before := terminalState(t, tty)
	output := startDrain(pty)
	stderr := tempFile(t)
	done := make(chan error, 1)
	go func() {
		done <- Run([]string{"--seed", "42", "--ascii", "--no-color", "--reduced-motion"}, tty, tty, stderr)
	}()

	waitFor(t, output, "NIGHTSHIFT")
	if _, err := pty.Write([]byte{'q', 0x1b, '\r'}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, output, burstLines(42, 0, PhaseBoot)[0])
	select {
	case err := <-done:
		t.Fatalf("non-ctrl+c input exited: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	secret := []byte("ZZ-SECRET-KEY")
	prior := len(output.Bytes())
	if _, err := pty.Write(secret); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(output.Bytes()) < prior+100 {
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("typed input exited: %v", err)
	default:
	}
	seen := output.Bytes()
	if len(seen) < prior+100 {
		t.Fatal("typed input did not append a burst")
	}
	if bytes.Contains(seen, secret) {
		t.Fatal("input bytes were written to the terminal")
	}
	for _, banned := range [][]byte{
		[]byte("\x1b[?1049"),
		[]byte("\x1b[?47"),
		[]byte("\x1b[2J"),
		[]byte("\x1b[3J"),
		[]byte("\x1b[H"),
		[]byte("LOCAL SIMULATION"),
	} {
		if bytes.Contains(seen, banned) {
			t.Fatalf("scrollback control %q in output", banned)
		}
	}
	if bytes.Count(seen, []byte("SEED 000000000000002A")) != 1 {
		t.Fatalf("header was not printed once: %q", seen)
	}
	if _, err := pty.Write([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ctrl+c did not exit")
	}
	waitFor(t, output, "\x1b[?2004l")
	if bytes.Count(output.Bytes(), []byte("\x1b[?2004h")) != 1 {
		t.Fatal("bracketed paste mode was not enabled once")
	}
	after := terminalState(t, tty)
	if *before != *after {
		t.Fatal("ctrl+c did not restore terminal state")
	}
}

func TestPTYDashboardTicksWhileWaitingAndCtrlCExits(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	pty, tty := openPTY(t)
	defer pty.Close()
	defer tty.Close()
	output := startDrain(pty)
	stderr := tempFile(t)
	done := make(chan error, 1)
	go func() {
		done <- Run([]string{"--seed", "42", "--ascii", "--no-color", "--reduced-motion"}, tty, tty, stderr)
	}()
	waitFor(t, output, "LIVE TELEMETRY")
	waitFor(t, output, "\x1b[4A\r")
	if _, err := pty.Write([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ctrl+c did not exit after a telemetry tick")
	}
}

func TestPTYEOFExitsRun(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	pty, tty := openPTY(t)
	defer tty.Close()
	output := startDrain(pty)
	stderr := tempFile(t)
	done := make(chan error, 1)
	go func() {
		done <- Run([]string{"--seed", "42", "--ascii", "--no-color", "--reduced-motion"}, tty, tty, stderr)
	}()
	waitFor(t, output, "LIVE TELEMETRY")
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run remained blocked after terminal EOF")
	}
}

func openPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pty.SyscallConn()
	if err != nil {
		pty.Close()
		t.Fatal(err)
	}
	var controlErr error
	var name string
	err = conn.Control(func(fd uintptr) {
		if controlErr = unix.IoctlSetInt(int(fd), unix.TIOCPTYGRANT, 0); controlErr != nil {
			return
		}
		if controlErr = unix.IoctlSetInt(int(fd), unix.TIOCPTYUNLK, 0); controlErr != nil {
			return
		}
		buf := make([]byte, 128)
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&buf[0])))
		if errno != 0 {
			controlErr = errno
			return
		}
		if end := bytes.IndexByte(buf, 0); end >= 0 {
			buf = buf[:end]
		}
		name = string(buf)
	})
	if err != nil || controlErr != nil || name == "" {
		pty.Close()
		t.Fatalf("pty setup: %v %v %q", err, controlErr, name)
	}
	tty, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		pty.Close()
		t.Fatal(err)
	}
	winsize := &unix.Winsize{Row: 40, Col: 120}
	if err := unix.IoctlSetWinsize(int(tty.Fd()), unix.TIOCSWINSZ, winsize); err != nil {
		tty.Close()
		pty.Close()
		t.Fatal(err)
	}
	return pty, tty
}

func terminalState(t *testing.T, tty *os.File) *term.State {
	t.Helper()
	state, err := term.GetState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func tempFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "nightshift-stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func startDrain(pty *os.File) *lockedBuffer {
	buffer := &lockedBuffer{}
	go func() {
		tmp := make([]byte, 4096)
		for {
			n, err := pty.Read(tmp)
			if n > 0 {
				buffer.Write(tmp[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return buffer
}

func waitFor(t *testing.T, buffer *lockedBuffer, needle string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(buffer.Bytes(), []byte(needle)) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; output %q", needle, buffer.Bytes())
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) {
	b.mu.Lock()
	b.buf.Write(p)
	b.mu.Unlock()
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}
