package nightshift

import (
	"fmt"
	"hash/fnv"
	"io"
	"strings"
	"time"
)

// Write-time motion. Nothing here touches the transcript; it only decides how
// long the writer waits and what the current line shows while it waits.
// Timing is uneven on purpose: commands take a moment to start, loaders stall
// and jump, and each kind of output streams at its own pace.

// effect is a write-time animation that settles on the rendered line.
// delays[i] is how long frames[i] stays up.
type effect struct {
	frames []string
	delays []time.Duration
}

func (fx *effect) add(frame string, delay time.Duration) {
	fx.frames = append(fx.frames, frame)
	fx.delays = append(fx.delays, delay)
}

const (
	scrambleGlyphs = "!<>-_/=+*^?#$%&@"
	scrambleSteps  = 16
	scrambleFrame  = 45 * time.Millisecond
	wordmarkPause  = 110 * time.Millisecond
	spinnerFrame   = 80 * time.Millisecond
	// A wait this long shows a spinner instead of a frozen line.
	spinnerAfter = 350 * time.Millisecond
)

var asciiSpinner = []string{"|", "/", "-", "\\"}

func millis(n int) time.Duration { return time.Duration(n) * time.Millisecond }

// paceFor seeds the pacing of one line from its text and the line before it,
// so the same output always moves the same way.
func paceFor(prev, line string) *rng {
	h := fnv.New64a()
	_, _ = io.WriteString(h, prev+"\n"+line)
	return &rng{state: h.Sum64()}
}

// waitBefore is how long a line takes to arrive after the previous one.
func waitBefore(prev, line string, r *rng) time.Duration {
	body := strings.TrimPrefix(line, fieldIndent)
	first := ""
	if words := strings.Fields(body); len(words) > 0 {
		first = words[0]
	}
	switch {
	case line == "", strings.HasPrefix(line, promptPrefix), wordmarkRow(line) >= 0:
		return 0
	case strings.HasPrefix(prev, promptPrefix):
		// The command runs before it prints anything.
		return millis(r.between(150, 1400))
	case strings.Contains(line, "timed out"):
		return millis(r.between(1800, 3200))
	case strings.HasPrefix(body, "retry "):
		return millis(r.between(700, 1600))
	case strings.HasSuffix(line, "* * *"):
		return millis(r.between(1400, 2600))
	case strings.HasSuffix(line, " ms") && len(first) <= 2:
		// A traceroute hop takes about as long as its round trip, stretched.
		var latency float64
		fmt.Sscanf(line[strings.LastIndex(strings.TrimSuffix(line, " ms"), " ")+1:], "%f", &latency)
		return millis(min(120+int(latency*18), 1600) + r.between(0, 200))
	case strings.HasSuffix(first, "/tcp") || strings.HasSuffix(first, "/udp"):
		return millis(r.between(150, 900))
	case isClock(first) && strings.Contains(line, " > "):
		// Packets arrive when they arrive.
		return millis(r.between(60, 1300))
	case isClock(first):
		// Logs and timelines dump fast, with the odd hitch.
		if r.intn(7) == 0 {
			return millis(r.between(250, 700))
		}
		return millis(r.between(15, 90))
	case strings.HasPrefix(body, "/") && (strings.HasSuffix(line, ": OK") || strings.HasSuffix(line, ": FAILED")):
		return millis(r.between(90, 800))
	case strings.HasPrefix(body, "0x"):
		return millis(r.between(20, 60))
	case strings.HasPrefix(line, okPrefix), strings.HasPrefix(line, warnPrefix):
		return millis(r.between(120, 600))
	case strings.HasPrefix(line, progressPrefix):
		return millis(r.between(60, 300))
	default:
		return millis(r.between(30, 220))
	}
}

// loaderBudget is roughly how long a loader takes to fill.
func loaderBudget(label string, r *rng) int {
	switch label {
	case "syn sweep", "disk image":
		return r.between(2200, 4200)
	case "memory image", "compress", "yara scan", "memory map":
		return r.between(1500, 3200)
	case "tls handshake", "pcap attach":
		return r.between(250, 800)
	case "rule push", "policy push", "report seal":
		return r.between(600, 1600)
	default:
		return r.between(400, 1500)
	}
}

func loaderEffect(line string, st style, r *rng) effect {
	body := line[len(progressPrefix):]
	label := strings.TrimRight(body[:strings.LastIndex(body, "[")], " ")
	perCell := loaderBudget(label, r) / progressCells
	var fx effect
	spin := 0
	for filled := 0; filled < progressCells; {
		frame := renderLine(progressFrame(label, filled), st)
		step := 1
		if r.intn(4) == 0 {
			step = r.between(2, 4)
		}
		delay := perCell * step * r.between(40, 170) / 100
		if r.intn(10) == 0 {
			// A stall: the bar holds while the spinner keeps turning.
			for hold := r.between(300, 1400); hold > 0; hold -= int(spinnerFrame / time.Millisecond) {
				fx.add(respin(frame, filled, spin, st), spinnerFrame)
				spin++
			}
		}
		fx.add(respin(frame, filled, spin, st), millis(delay))
		spin++
		filled = min(filled+step, progressCells)
	}
	return fx
}

// respin swaps the loader's spinner glyph so it turns even while a bar holds.
func respin(frame string, filled, turn int, st style) string {
	if !st.unicode {
		return frame
	}
	return strings.Replace(frame, spinner[filled%len(spinner)], spinner[turn%len(spinner)], 1)
}

// typingEffect types a command with a human rhythm: a beat before the first
// key, uneven keystrokes, and small hesitations at word breaks.
func typingEffect(line string, st style, r *rng) effect {
	cmd := line[len(promptPrefix):]
	caret := "_"
	if st.unicode {
		caret = "▌"
	}
	caret = join([]segment{{caret, ansiPrompt}}, st.color)
	var fx effect
	fx.add(renderLine(promptPrefix, st)+caret, millis(r.between(200, 700)))
	for i := 1; i <= len(cmd); i++ {
		delay := r.between(18, 75)
		switch {
		case cmd[i-1] == ' ' && r.intn(3) == 0:
			delay += r.between(80, 320)
		case r.intn(12) == 0:
			delay = r.between(6, 12) // a quick run of keys
		}
		fx.add(renderLine(promptPrefix+cmd[:i], st)+caret, millis(delay))
	}
	return fx
}

func scrambleEffect(line string, st style) effect {
	body := line[len("+==[ "):]
	end := strings.Index(body, " ]")
	if end < 0 {
		return effect{}
	}
	label := body[:end]
	var fx effect
	for step := 0; step < scrambleSteps; step++ {
		settled := step * len(label) / scrambleSteps
		noise := []byte(label)
		for i := settled; i < len(noise); i++ {
			if noise[i] != ' ' {
				pick := splitmix(uint64(step)<<16|uint64(i)) % uint64(len(scrambleGlyphs))
				noise[i] = scrambleGlyphs[pick]
			}
		}
		fx.add(renderLine(strings.Replace(line, label, string(noise), 1), st), scrambleFrame)
	}
	return fx
}

func glitchEffect(line string, st style) effect {
	seed := paceFor("glitch", line).next()
	var positions []int
	for index, char := range []byte(line) {
		if char != ' ' && char != '\t' {
			positions = append(positions, index)
		}
	}
	var fx effect
	const steps = 7
	for step := 0; step < steps; step++ {
		resolved := step * len(positions) / steps
		frame := []byte(line)
		for index := resolved; index < len(positions); index++ {
			at := positions[index]
			pick := splitmix(seed^uint64(step)<<16^uint64(at)) % uint64(len(scrambleGlyphs))
			frame[at] = scrambleGlyphs[pick]
		}
		fx.add(renderLine(string(frame), st), millis(26))
	}
	return fx
}

func waveformEffect(st style) effect {
	var fx effect
	for stage, delay := range []time.Duration{millis(90), millis(90), millis(100), millis(110)} {
		pattern := waveformText(stage, st.unicode)
		frame := join([]segment{{"  SIGNAL ", ansiDim}, {pattern, ansiBrightGreen}}, st.color)
		fx.add(frame, delay)
	}
	return fx
}

func shouldGlitch(prev, line string) bool {
	if line == "" || strings.HasPrefix(line, promptPrefix) || strings.HasPrefix(line, progressPrefix) || strings.HasPrefix(line, traceMapPrefix) || line == waveformLine {
		return false
	}
	r := paceFor(prev, line)
	return r.intn(37) == 0
}

// waitEffect fills a long wait with a spinner on the line about to print.
func waitEffect(wait time.Duration, st style) effect {
	var fx effect
	if wait < spinnerAfter {
		if wait > 0 {
			fx.add("", wait)
		}
		return fx
	}
	glyphs := spinner[:]
	if !st.unicode {
		glyphs = asciiSpinner
	}
	for turn := 0; wait > 0; turn++ {
		step := min(spinnerFrame, wait)
		fx.add("   "+join([]segment{{glyphs[turn%len(glyphs)], ansiPrompt}}, st.color), step)
		wait -= step
	}
	return fx
}

func effectFor(prev, line string, st style) effect {
	r := paceFor(prev, line)
	fx := waitEffect(waitBefore(prev, line, r), st)
	var own effect
	switch {
	case strings.HasPrefix(line, progressPrefix):
		own = loaderEffect(line, st, r)
	case strings.HasPrefix(line, promptPrefix):
		own = typingEffect(line, st, r)
	case strings.HasPrefix(line, "+==[ "):
		own = scrambleEffect(line, st)
	case line == waveformLine:
		own = waveformEffect(st)
	case shouldGlitch(prev, line):
		own = glitchEffect(line, st)
	}
	fx.frames = append(fx.frames, own.frames...)
	fx.delays = append(fx.delays, own.delays...)
	return fx
}

// linePause holds after a wordmark row so the wordmark scans in.
func linePause(line string) time.Duration {
	if wordmarkRow(line) >= 0 {
		return wordmarkPause
	}
	return 0
}

// renderLines writes new lines only. Effects redraw the current line with a
// carriage return; earlier lines are never touched.
func renderLines(output io.Writer, lines []string, st style) error {
	var pending strings.Builder
	flush := func() error {
		if pending.Len() == 0 {
			return nil
		}
		_, err := io.WriteString(output, pending.String())
		pending.Reset()
		return err
	}
	hidden := false
	prev := ""
	for _, line := range lines {
		if strings.HasPrefix(line, traceMapPrefix) {
			if err := flush(); err != nil {
				return err
			}
			if err := renderTraceMap(output, line, st); err != nil {
				return err
			}
			prev = line
			continue
		}
		final := renderLine(line, st)
		if st.bell && strings.HasPrefix(line, warnPrefix) {
			pending.WriteByte('\a')
		}
		if !st.motion {
			pending.WriteString(final + "\r\n")
			continue
		}
		fx := effectFor(prev, line, st)
		prev = line
		if len(fx.frames) == 0 {
			pending.WriteString(final + "\r\n")
		} else {
			if !hidden {
				pending.WriteString(hideCursor)
				hidden = true
			}
			for i, frame := range fx.frames {
				fmt.Fprintf(&pending, "\r%s%s", frame, eraseLine)
				if err := flush(); err != nil {
					return err
				}
				sleep(fx.delays[i])
			}
			fmt.Fprintf(&pending, "\r%s%s\r\n", final, eraseLine)
		}
		if pause := linePause(line); pause > 0 {
			if err := flush(); err != nil {
				return err
			}
			sleep(pause)
		}
	}
	if hidden {
		pending.WriteString(showCursor)
	}
	return flush()
}

func renderTraceMap(output io.Writer, directive string, st style) error {
	hops := parseTraceMap(directive)
	if len(hops) == 0 {
		return nil
	}
	if !st.motion {
		return writeLines(output, traceMapRows(hops, len(hops)))
	}
	first := true
	for active := 1; active <= len(hops); active++ {
		if !st.motion && active < len(hops) {
			continue
		}
		rows := traceMapRows(hops, active)
		if !first {
			if _, err := fmt.Fprintf(output, "\x1b[%dA", len(rows)); err != nil {
				return err
			}
		}
		for _, row := range rows {
			if _, err := fmt.Fprintf(output, "\r\x1b[2K%s\r\n", row); err != nil {
				return err
			}
		}
		first = false
		if st.motion && active < len(hops) {
			sleep(millis(125))
		}
	}
	return nil
}
