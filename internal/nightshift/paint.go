package nightshift

import (
	"strings"
	"unicode/utf8"
)

const (
	ansiReset       = "\x1b[0m"
	ansiDim         = "\x1b[2m"
	ansiFrame       = "\x1b[2;36m"
	ansiLabel       = "\x1b[1;36m"
	ansiBoldWhite   = "\x1b[1;37m"
	ansiPrompt      = "\x1b[1;32m"
	ansiBrightGreen = "\x1b[1;92m"
	ansiYellow      = "\x1b[1;33m"
	ansiMeter       = "\x1b[36m"
	ansiRoot        = "\x1b[1;38;5;203m"
	ansiAmber       = "\x1b[38;5;214m"
	ansiDeepGreen   = "\x1b[38;5;28m"
	ansiGood        = "\x1b[32m"
	ansiAlert       = "\x1b[31m"

	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	eraseLine  = "\x1b[K"
)

// wordmarkShades runs the five wordmark rows from cyan to blue.
var wordmarkShades = [5]string{
	"\x1b[1;38;5;51m",
	"\x1b[1;38;5;45m",
	"\x1b[1;38;5;39m",
	"\x1b[1;38;5;33m",
	"\x1b[1;38;5;27m",
}

// barShades color a loader from deep green to cyan along its length.
var barShades = [...]string{
	"\x1b[38;5;28m", "\x1b[38;5;34m", "\x1b[38;5;40m", "\x1b[38;5;46m",
	"\x1b[38;5;47m", "\x1b[38;5;48m", "\x1b[38;5;49m", "\x1b[38;5;50m", "\x1b[38;5;51m",
}

var spinner = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// style is how stored lines are written. The transcript itself stays plain
// ASCII; glyphs, color, and motion are applied at write time only.
type style struct {
	color    bool
	unicode  bool
	motion   bool
	bell     bool
	callsign string
}

func styleFor(config Config, callsign string) style {
	return style{
		color:    !config.NoColor,
		unicode:  !config.ASCII,
		motion:   !config.ReducedMotion,
		bell:     config.Bell,
		callsign: strings.ToLower(callsign),
	}
}

// segment is one run of rendered text. An empty sgr uses the default color.
type segment struct {
	text string
	sgr  string
}

// renderLine writes one stored line in the given style. With neither color
// nor unicode it returns the line unchanged.
func renderLine(line string, st style) string {
	return join(segmentsFor(line, st), st.color)
}

func join(segs []segment, color bool) string {
	var b strings.Builder
	for _, seg := range segs {
		if color && seg.sgr != "" && seg.text != "" {
			b.WriteString(seg.sgr)
			b.WriteString(seg.text)
			b.WriteString(ansiReset)
			continue
		}
		b.WriteString(seg.text)
	}
	return b.String()
}

func segmentsFor(line string, st style) []segment {
	uni := st.unicode
	switch {
	case line == "":
		return nil
	case strings.HasPrefix(line, "+==[ "):
		return borderTopSegments(line, uni)
	case strings.HasPrefix(line, "+==") && strings.Trim(line, "+=") == "":
		if uni {
			return []segment{{"╰" + strings.Repeat("─", len(line)-2) + "╯", ansiFrame}}
		}
		return []segment{{line, ansiFrame}}
	case strings.HasPrefix(line, "|== ") && strings.HasSuffix(line, " ==|"):
		return titleSegments(line, uni)
	case strings.HasPrefix(line, promptPrefix):
		return promptSegments(line, st)
	case strings.HasPrefix(line, okPrefix):
		return statusSegments(line, okPrefix, "[ok]", "✓", ansiBrightGreen, uni)
	case strings.HasPrefix(line, warnPrefix):
		return statusSegments(line, warnPrefix, "[warn]", "▲", ansiYellow, uni)
	case strings.HasPrefix(line, radioPrefix):
		return []segment{{radioPrefix, ansiLabel}, {line[len(radioPrefix):], ansiDim}}
	case strings.HasPrefix(line, "THREATCON "):
		return threatSegments(line, uni)
	case line == waveformLine:
		return []segment{{"  SIGNAL ", ansiDim}, {waveformText(3, uni), ansiBrightGreen}}
	case strings.HasPrefix(line, progressPrefix):
		return progressSegments(line, uni)
	case strings.HasPrefix(line, "// "):
		return []segment{{"//", ansiDeepGreen}, {line[2:], ansiDim}}
	case strings.HasPrefix(line, fieldIndent+"0x"):
		return hexSegments(line, uni)
	case strings.HasPrefix(line, fieldIndent):
		return fieldSegments(line, uni)
	default:
		return []segment{{line, ""}}
	}
}

func threatSegments(line string, uni bool) []segment {
	body := strings.TrimPrefix(line, "THREATCON ")
	space := strings.IndexByte(body, ' ')
	if space < 0 {
		return []segment{{line, ansiYellow}}
	}
	bar, level := body[:space], body[space+1:]
	if uni {
		bar = strings.NewReplacer("#", "▰", "-", "▱").Replace(bar)
	}
	indicator := ansiGood
	switch level {
	case "ELEVATED":
		indicator = ansiYellow
	case "HIGH":
		indicator = ansiAmber
	case "CRITICAL":
		indicator = ansiAlert
	}
	return []segment{{"THREATCON ", ansiDim}, {bar, indicator}, {" " + level, indicator}}
}

func borderTopSegments(line string, uni bool) []segment {
	body := strings.TrimPrefix(line, "+==[ ")
	end := strings.Index(body, " ]")
	if end < 0 {
		return []segment{{line, ansiFrame}}
	}
	label := body[:end]
	if !uni {
		return []segment{{"+==[ ", ansiFrame}, {label, ansiLabel}, {body[end:], ansiFrame}}
	}
	// "╭─ " + label + " " + rule + "╮" keeps the ASCII line's width.
	rule := len(line) - utf8.RuneCountInString(label) - 5
	if rule < 1 {
		rule = 1
	}
	return []segment{{"╭─ ", ansiFrame}, {label, ansiLabel}, {" " + strings.Repeat("─", rule) + "╮", ansiFrame}}
}

func titleSegments(line string, uni bool) []segment {
	left, right := "|== ", " ==|"
	inner := line[len(left) : len(line)-len(right)]
	trim := strings.TrimSpace(inner)
	var middle []segment
	switch {
	case wordmarkRow(line) >= 0:
		if uni {
			inner = strings.ReplaceAll(inner, "#", "█")
		}
		middle = []segment{{inner, wordmarkShades[wordmarkRow(line)]}}
	case trim != "" && strings.Trim(trim, "=") == "":
		if uni {
			inner = strings.ReplaceAll(inner, "=", "─")
		}
		middle = []segment{{inner, ansiFrame}}
	case strings.Contains(inner, "SEED ") && strings.Contains(inner, "CALLSIGN "):
		middle = seedSegments(inner)
	default:
		middle = []segment{{inner, ""}}
	}
	if uni {
		left, right = "│   ", "   │"
	}
	return append(append([]segment{{left, ansiFrame}}, middle...), segment{right, ansiFrame})
}

// seedSegments splits "  SEED <hex>    CALLSIGN <name>  " into colored runs.
func seedSegments(inner string) []segment {
	hexAt := strings.Index(inner, "SEED ") + len("SEED ")
	hexEnd := hexAt + strings.IndexByte(inner[hexAt:], ' ')
	callAt := strings.Index(inner, "CALLSIGN ")
	nameAt := callAt + len("CALLSIGN ")
	nameEnd := nameAt + strings.IndexByte(inner[nameAt:]+" ", ' ')
	return []segment{
		{inner[:hexAt-len("SEED ")], ""},
		{"SEED ", ansiDim},
		{inner[hexAt:hexEnd], ansiAmber},
		{inner[hexEnd:callAt], ""},
		{"CALLSIGN ", ansiDim},
		{inner[nameAt:nameEnd], ansiRoot},
		{inner[nameEnd:], ""},
	}
}

var wordmarkLines = func() map[string]int {
	lines := map[string]int{}
	for i, row := range wordmarkRows() {
		lines[titleInner(centerText(row, screenWidth-8))] = i
	}
	return lines
}()

func wordmarkRow(line string) int {
	if row, ok := wordmarkLines[line]; ok {
		return row
	}
	return -1
}

// promptSegments renders "nightshift> cmd". Unicode mode dresses the prompt
// as callsign@nightshift.
func promptSegments(line string, st style) []segment {
	cmd := line[len(promptPrefix)-1:]
	if !st.unicode {
		return []segment{{"nightshift>", ansiPrompt}, {cmd, ""}}
	}
	if st.callsign == "" {
		return []segment{{" ❯", ansiPrompt}, {cmd, ansiBoldWhite}}
	}
	return []segment{
		{" " + st.callsign, ansiRoot},
		{"@", ansiDim},
		{"nightshift", ansiLabel},
		{" ❯", ansiPrompt},
		{cmd, ansiBoldWhite},
	}
}

func statusSegments(line, prefix, tag, glyph, sgr string, uni bool) []segment {
	rest := highlight(line[len(prefix):])
	if uni {
		return append([]segment{{"   ", ""}, {glyph, sgr}, {" ", ""}}, rest...)
	}
	return append([]segment{{tag, sgr}, {prefix[len(tag):], ""}}, rest...)
}

// progressSegments renders "[>>]   label [####----] NN%". A loader that is
// still filling shows a spinner in unicode mode.
func progressSegments(line string, uni bool) []segment {
	body := line[len(progressPrefix):]
	open := strings.LastIndex(body, "[")
	shut := strings.LastIndex(body, "]")
	if open < 0 || shut < open {
		return []segment{{line, ""}}
	}
	label, bars, pct := body[:open], body[open+1:shut], body[shut+1:]
	filled := strings.Count(bars, "#")
	var segs []segment
	if uni {
		glyph, sgr := "▸", ansiLabel
		if filled < len(bars) {
			glyph, sgr = spinner[filled%len(spinner)], ansiPrompt
		}
		segs = append(segs, segment{"   ", ""}, segment{glyph, sgr}, segment{" " + label, ansiDim})
	} else {
		segs = append(segs, segment{"[>>]", ansiLabel}, segment{progressPrefix[4:] + label + "[", ansiDim})
	}
	cell, hole := "#", "-"
	if uni {
		cell, hole = "█", "░"
	}
	for i := 0; i < filled; i++ {
		segs = append(segs, segment{cell, barShades[i*len(barShades)/len(bars)]})
	}
	segs = append(segs, segment{strings.Repeat(hole, len(bars)-filled), ansiDim})
	if !uni {
		segs = append(segs, segment{"]", ansiDim})
	}
	pctSgr := ansiDim
	if filled == len(bars) {
		pctSgr = ansiBrightGreen
	}
	return append(segs, segment{pct, pctSgr})
}

// hexSegments renders "indent 0xOFFS  hh hh ..  |gutter|". The offset and
// gutter recede so the bytes read as one block.
func hexSegments(line string, uni bool) []segment {
	indent := fieldIndent
	if uni {
		indent = "     "
	}
	parts := strings.SplitN(line[len(fieldIndent):], "  ", 3)
	if len(parts) != 3 {
		return []segment{{indent + line[len(fieldIndent):], ""}}
	}
	segs := []segment{{indent, ""}, {parts[0], ansiDim}, {" ", ""}}
	for _, pair := range strings.Fields(parts[1]) {
		sgr := ""
		if pair == "00" {
			sgr = ansiDim
		}
		segs = append(segs, segment{" ", ""}, segment{pair, sgr})
	}
	gutter := parts[2]
	if uni {
		gutter = strings.ReplaceAll(gutter, "|", "│")
	}
	return append(segs, segment{"  ", ""}, segment{gutter, ansiDim})
}

// fieldSegments renders a detail line. A meter's value is its bar; a leading
// lowercase label followed by two spaces is dimmed; the rest is highlighted.
func fieldSegments(line string, uni bool) []segment {
	body := line[len(fieldIndent):]
	indent := fieldIndent
	if uni {
		indent = "     "
	}
	if open := strings.LastIndex(body, "["); open >= 0 && strings.HasSuffix(body, "]") {
		bars := body[open+1 : len(body)-1]
		filled := strings.Count(bars, "#")
		if filled+strings.Count(bars, "-") == len(bars) {
			empty := len(bars) - filled
			if uni {
				return []segment{{indent, ""}, {body[:open], ansiDim},
					{strings.Repeat("▰", filled), ansiMeter}, {strings.Repeat("▱", empty), ansiDim}}
			}
			return []segment{{indent, ""}, {body[:open+1], ansiDim},
				{bars[:filled], ansiMeter}, {bars[filled:] + "]", ansiDim}}
		}
	}
	segs := []segment{{indent, ""}}
	if header(body) {
		return append(segs, segment{body, ansiBoldWhite})
	}
	if label, value, ok := strings.Cut(body, "  "); ok && isLabel(label) {
		gap := len(value) - len(strings.TrimLeft(value, " "))
		segs = append(segs, segment{label + "  " + value[:gap], ansiDim})
		return append(segs, highlight(value[gap:])...)
	}
	return append(segs, highlight(body)...)
}

func isLabel(word string) bool {
	if word == "" || word[0] < 'a' || word[0] > 'z' {
		return false
	}
	for i := 0; i < len(word); i++ {
		if !(word[i] >= 'a' && word[i] <= 'z' || word[i] >= '0' && word[i] <= '9') {
			return false
		}
	}
	return true
}

// header reports a table heading such as "PORT  STATE  SERVICE".
func header(body string) bool {
	words := strings.Fields(body)
	if len(words) < 2 {
		return false
	}
	for _, word := range words {
		if strings.Trim(word, "ABCDEFGHIJKLMNOPQRSTUVWXYZ%") != "" {
			return false
		}
	}
	return true
}

var (
	goodWords = wordSet("open", "OK", "Accepted", "verified", "good", "sealed", "up", "loaded", "blocked", "match")
	badWords  = wordSet("Failed", "FAILED", "DROP", "NOT", "unsigned", "quarantined", "WARNING:")
	dimWords  = wordSet("closed", "-", "*", "ms", "msec", "IN", "->")
)

func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

// highlight colors each token of a realistic console line and leaves the
// spacing between tokens untouched.
func highlight(text string) []segment {
	var segs []segment
	for i := 0; i < len(text); {
		j := i
		space := text[i] == ' '
		for j < len(text) && (text[j] == ' ') == space {
			j++
		}
		token := text[i:j]
		if space {
			segs = append(segs, segment{token, ""})
		} else {
			segs = append(segs, segment{token, shade(token)})
		}
		i = j
	}
	return segs
}

// shade keeps color for outcomes only: green is good, red is bad, and
// clocks and digests recede. Addresses, hosts, and paths stay plain.
func shade(token string) string {
	core := strings.Trim(token, "()[],;:'\"")
	switch {
	case goodWords[core] || goodWords[token]:
		return ansiGood
	case badWords[core] || badWords[token]:
		return ansiAlert
	case dimWords[core] || dimWords[token], isClock(core), isHexish(core), strings.HasPrefix(core, "SHA256:"):
		return ansiDim
	}
	return ""
}

func isClock(word string) bool {
	if len(word) < 8 || word[2] != ':' || word[5] != ':' {
		return false
	}
	return strings.Trim(word, "0123456789:.") == ""
}

func isHexish(word string) bool {
	word = strings.TrimSuffix(word, "...")
	if len(word) < 12 {
		return false
	}
	return strings.Trim(word, "0123456789abcdefABCDEF:") == ""
}
