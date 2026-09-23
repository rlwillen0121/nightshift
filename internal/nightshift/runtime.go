package nightshift

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// ErrInteractiveTerminal is reported when stdin or stdout is not a terminal.
var ErrInteractiveTerminal = errors.New("interactive terminal required")

// Run owns only terminal I/O and local presentation. It does not discover or
// invoke processes, access the network, inspect files, or persist state.
//
// The session prints a short header once, then writes only new lines. Raw
// mode leaves scrollback to the terminal. The alternate screen is not used.
// Operator key bytes are classified and discarded.
func Run(args []string, input, output, errorOutput *os.File) error {
	config, err := ParseArgs(args, os.Getenv)
	if err != nil {
		return ExitError{Code: 2, Message: err.Error()}
	}
	if config.Help {
		if output == nil {
			return ErrInteractiveTerminal
		}
		_, err := io.WriteString(output, Usage())
		return err
	}
	if input == nil || output == nil || errorOutput == nil || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return ErrInteractiveTerminal
	}
	fd := int(input.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("terminal session unavailable")
	}
	defer func() {
		_, _ = io.WriteString(output, showCursor+"\x1b[?2004l")
		_ = term.Restore(fd, state)
	}()
	if _, err = io.WriteString(output, "\x1b[?2004h"); err != nil {
		return fmt.Errorf("terminal session unavailable")
	}

	model := NewModel(config)
	st := styleFor(model.config, model.cosmetic.Callsign)
	columns, _, sizeErr := term.GetSize(int(output.Fd()))
	repaintSafe := sizeErr == nil && terminalRepaintFits(columns)
	if !repaintSafe {
		st.motion = false
	}
	liveDashboard := !config.DumbTerminal && repaintSafe
	// Glyphs, color, and motion are applied at write time; stored lines stay ASCII.
	if err = renderLines(output, model.transcript, st); err != nil {
		return fmt.Errorf("terminal session unavailable")
	}
	if err = drawDashboard(output, dashboardLines(model), st, false); err != nil {
		return fmt.Errorf("terminal session unavailable")
	}
	// Typing speed sets playback speed. Bytes are timed as they arrive, both
	// here and while an effect is playing.
	pace := &tempo{}
	reader := &byteReader{
		f:       input,
		wait:    func(timeout time.Duration) (bool, error) { return waitForInput(input, timeout) },
		arrived: func(chunk []byte) { pace.hit(time.Now(), typedKeys(chunk)) },
	}
	st.pace = (&pacer{tempo: pace, reader: reader, poll: reader.wait, clock: time.Now}).wait
	dashboardVisible := liveDashboard
	for {
		if !reader.buffered() && liveDashboard && !model.finale {
			ready, waitErr := waitForInput(input, tickEvery)
			if errors.Is(waitErr, errInputPollingUnavailable) {
				liveDashboard = false
				dashboardVisible = false
			} else if waitErr != nil {
				return fmt.Errorf("terminal session unavailable")
			} else if !ready {
				model = model.Tick(Now())
				if err = drawDashboard(output, dashboardLines(model), st, true); err != nil {
					return fmt.Errorf("terminal session unavailable")
				}
				continue
			}
		}

		kind, readErr := reader.nextKey()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return fmt.Errorf("terminal session unavailable")
		}
		if kind == keyQuit {
			return nil
		}
		if kind == keyIgnore {
			continue
		}
		if dashboardVisible && liveDashboard {
			if err = eraseDashboard(output, len(dashboardLines(model))); err != nil {
				return fmt.Errorf("terminal session unavailable")
			}
			dashboardVisible = false
		}
		model = model.Tick(Now())
		var lines []string
		var quit bool
		model, lines, quit = model.Handle(kind)
		if quit {
			return nil
		}
		if err = renderLines(output, lines, st); err != nil {
			return fmt.Errorf("terminal session unavailable")
		}
		if !model.finale && liveDashboard {
			if err = drawDashboard(output, dashboardLines(model), st, false); err != nil {
				return fmt.Errorf("terminal session unavailable")
			}
			dashboardVisible = true
		}
	}
}

func terminalRepaintFits(columns int) bool { return columns >= screenWidth }

func drawDashboard(output io.Writer, lines []string, st style, repaint bool) error {
	if repaint {
		if _, err := fmt.Fprintf(output, "\x1b[%dA\r", len(lines)); err != nil {
			return err
		}
	}
	for _, line := range lines {
		prefix := ""
		if repaint {
			prefix = "\r\x1b[2K"
		}
		if _, err := fmt.Fprintf(output, "%s%s\r\n", prefix, renderLine(line, st)); err != nil {
			return err
		}
	}
	return nil
}

func eraseDashboard(output io.Writer, rows int) error {
	if _, err := fmt.Fprintf(output, "\x1b[%dA\r", rows); err != nil {
		return err
	}
	for range rows {
		if _, err := io.WriteString(output, "\x1b[2K\r\n"); err != nil {
			return err
		}
	}
	return nil
}

func writeLines(output io.Writer, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(line)
		builder.WriteString("\r\n")
	}
	_, err := io.WriteString(output, builder.String())
	return err
}
