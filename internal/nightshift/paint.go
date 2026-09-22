package nightshift

import (
	"io"
	"strings"
)

const (
	ansiReset       = "\x1b[0m"
	ansiBoldCyan    = "\x1b[1;36m"
	ansiBoldWhite   = "\x1b[1;37m"
	ansiGreen       = "\x1b[32m"
	ansiBrightGreen = "\x1b[1;92m"
	ansiYellow      = "\x1b[1;33m"
	ansiDim         = "\x1b[2m"
	ansiBoldMagenta = "\x1b[1;35m"
)

// paintLine colors one stored line. The transcript itself stays plain text.
func paintLine(line string) string {
	if line == "" {
		return ""
	}
	if isTitleLine(line) {
		if strings.Contains(line, "SEED ") {
			return ansiBoldWhite + line + ansiReset
		}
		return ansiBoldCyan + line + ansiReset
	}
	trim := strings.TrimSpace(line)
	if strings.HasPrefix(trim, "K.I.K.I.") {
		return ansiBoldMagenta + line + ansiReset
	}
	if strings.HasPrefix(trim, "nightshift>") {
		return ansiGreen + line + ansiReset
	}
	if idx := strings.Index(line, "//"); idx >= 0 && strings.TrimSpace(line[:idx]) == "" {
		return ansiDim + line + ansiReset
	}
	if idx := strings.Index(line, "//"); idx >= 0 {
		return paintTags(line[:idx]) + ansiDim + line[idx:] + ansiReset
	}
	return paintTags(line)
}

func isTitleLine(line string) bool {
	return strings.HasPrefix(line, "+==") || strings.HasPrefix(line, "|==")
}

func paintTags(line string) string {
	var b strings.Builder
	b.WriteString(ansiGreen)
	rest := line
	for len(rest) > 0 {
		okAt := strings.Index(rest, "[ok]")
		warnAt := strings.Index(rest, "[warn]")
		at, tag := -1, ""
		if okAt >= 0 && (warnAt < 0 || okAt < warnAt) {
			at, tag = okAt, "[ok]"
		} else if warnAt >= 0 {
			at, tag = warnAt, "[warn]"
		}
		if at < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:at])
		if tag == "[ok]" {
			b.WriteString(ansiBrightGreen)
		} else {
			b.WriteString(ansiYellow)
		}
		b.WriteString(tag)
		b.WriteString(ansiGreen)
		rest = rest[at+len(tag):]
	}
	b.WriteString(ansiReset)
	return b.String()
}

func renderLines(output io.Writer, lines []string, color bool) error {
	if !color {
		return writeLines(output, lines)
	}
	return writeColoredLines(output, lines)
}

func writeColoredLines(output io.Writer, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	painted := make([]string, len(lines))
	for i, line := range lines {
		painted[i] = paintLine(line)
	}
	return writeLines(output, painted)
}
