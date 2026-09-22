package nightshift

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	if input == nil || output == nil || errorOutput == nil || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return ErrInteractiveTerminal
	}
	fd := int(input.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("terminal session unavailable")
	}
	defer func() {
		_, _ = io.WriteString(output, "\x1b[?2004l")
		_ = term.Restore(fd, state)
	}()
	if _, err = io.WriteString(output, "\x1b[?2004h"); err != nil {
		return fmt.Errorf("terminal session unavailable")
	}

	model := NewModel(config)
	// Color is paint-at-write. NoColor stores and prints the same plain lines.
	if err = renderLines(output, model.transcript, !config.NoColor); err != nil {
		return fmt.Errorf("terminal session unavailable")
	}
	reader := &byteReader{f: input}
	for {
		kind, readErr := reader.nextKey()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return fmt.Errorf("terminal session unavailable")
		}
		var lines []string
		var quit bool
		model, lines, quit = model.Handle(kind)
		if quit {
			return nil
		}
		if err = renderLines(output, lines, !config.NoColor); err != nil {
			return fmt.Errorf("terminal session unavailable")
		}
	}
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
