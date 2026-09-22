package nightshift

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// fullRun walks every phase, the finale, and a restart for one seed.
func fullRun() []string {
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	model, _ = model.Enter()
	for phase := PhaseBoot; phase <= PhaseReport; phase++ {
		model, _ = model.Trigger()
		model, _ = model.Trigger()
		model, _ = model.Enter()
	}
	model, _ = model.Enter()
	return model.transcript
}

func TestPlainStyleIsTheStoredLine(t *testing.T) {
	for _, line := range fullRun() {
		if line == waveformLine {
			continue // the ASCII placeholder renders as the settled signal graph
		}
		if got := renderLine(line, style{}); got != line {
			t.Fatalf("plain render changed %q to %q", line, got)
		}
	}
}

func TestUnicodeGlyphs(t *testing.T) {
	uni := style{unicode: true}
	for _, test := range []struct{ in, want string }{
		{borderTop("01 BOOT ATTESTATION"), "╭─ 01 BOOT ATTESTATION " + strings.Repeat("─", 48) + "╮"},
		{borderBottom(), "╰" + strings.Repeat("─", 70) + "╯"},
		{titleInner("  Local envelope check."), "│     Local envelope check." + strings.Repeat(" ", 41) + "   │"},
		{"nightshift> seal --local", " ❯ seal --local"},
		{okPrefix + "web-01.ember.invalid  up", "   ✓ web-01.ember.invalid  up"},
		{warnPrefix + "pid 4127  unsigned binary", "   ▲ pid 4127  unsigned binary"},
		{field("addr", "192.0.2.44"), "     addr     192.0.2.44"},
		{meterLine("web-01", 3), "     web-01   ▰▰▰▱▱▱▱▱"},
		{detail("%-9s %-9s %s", "22/tcp", "open", "ssh"), "     22/tcp    open      ssh"},
		{progressLine("decrypt"), "   ▸ decrypt       " + strings.Repeat("█", progressCells) + " 100%"},
		{progressFrame("decrypt", 5), "   ⠴ decrypt       █████" + strings.Repeat("░", 15) + "  25%"},
		{hexRow(0x3fa0, []byte("beacon-v")), "     0x3fa0  62 65 61 63 6f 6e 2d 76  │beacon-v│"},
		{"// a key appends one burst.", "// a key appends one burst."},
		{"plain text", "plain text"},
	} {
		if got := renderLine(test.in, uni); got != test.want {
			t.Errorf("renderLine(%q)\n got %q\nwant %q", test.in, got, test.want)
		}
	}
	for _, line := range fullRun() {
		got := renderLine(line, uni)
		if strings.Contains(got, "·") {
			t.Fatal("middle dot returned")
		}
		if strings.HasPrefix(got, "╭") || strings.HasPrefix(got, "╰") || strings.HasPrefix(got, "│") {
			if n := utf8.RuneCountInString(got); n != screenWidth {
				t.Fatalf("frame line is %d wide: %q", n, got)
			}
		}
		if strings.Contains(got, "#") || strings.Contains(got, "|==") {
			t.Fatalf("ascii frame leaked into unicode: %q", got)
		}
	}
}

func TestStylesKeepTheirPromises(t *testing.T) {
	for _, line := range fullRun() {
		ascii := renderLine(line, style{color: true})
		for i := 0; i < len(ascii); i++ {
			if ascii[i] > 127 {
				t.Fatalf("ascii style emitted non-ascii: %q", ascii)
			}
		}
		if strings.Contains(renderLine(line, style{unicode: true}), "\x1b") {
			t.Fatalf("no-color style emitted an escape: %q", line)
		}
		painted := renderLine(line, style{color: true, unicode: true})
		if strings.Contains(painted, "\x1b[1;35m") {
			t.Fatalf("magenta returned: %q", painted)
		}
	}
	if !strings.Contains(renderLine(wordmarkLineFor(0), style{color: true, unicode: true}), wordmarkShades[0]) {
		t.Fatal("wordmark gradient missing")
	}
}

func TestPromptWearsTheCallsign(t *testing.T) {
	got := renderLine("nightshift> seal --local", style{unicode: true, callsign: "mothlight"})
	if got != " mothlight@nightshift ❯ seal --local" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestMotionRedrawsOnlyTheCurrentLine(t *testing.T) {
	var naps int
	saved := sleep
	sleep = func(time.Duration) { naps++ }
	defer func() { sleep = saved }()

	lines := append(NewModel(Config{Seed: 42, SeedProvided: true}).transcript, burstLines(42, 0, PhaseBoot)...)
	var moving, still bytes.Buffer
	if err := renderLines(&moving, lines, style{color: true, unicode: true, motion: true}); err != nil {
		t.Fatal(err)
	}
	if naps == 0 || !strings.Contains(moving.String(), hideCursor) || !strings.HasSuffix(moving.String(), showCursor) {
		t.Fatalf("motion did not animate: %d naps", naps)
	}
	for _, banned := range []string{"\x1b[?1049", "\x1b[2J", "\x1b[H", "\x1b[A", "\x1b[1A"} {
		if strings.Contains(moving.String(), banned) {
			t.Fatalf("motion moved the cursor off the current line: %q", banned)
		}
	}
	naps = 0
	if err := renderLines(&still, lines, style{color: true, unicode: true}); err != nil {
		t.Fatal(err)
	}
	if naps != 0 || strings.Contains(still.String(), hideCursor) || strings.Contains(strings.ReplaceAll(still.String(), "\r\n", ""), "\r") {
		t.Fatal("reduced motion still animated")
	}
	// Every animated line settles on exactly what reduced motion prints.
	settled := strings.Split(strings.ReplaceAll(still.String(), "\r\n", "\n"), "\n")
	for _, line := range settled {
		if !strings.Contains(moving.String(), line) {
			t.Fatalf("motion never settled on %q", line)
		}
	}
}

func TestHighlighterKeepsColorForOutcomes(t *testing.T) {
	color := style{color: true, unicode: true}
	auth := detail("02:14:07 sshd[4127]: Failed password for root from 198.51.100.201")
	scan := detail("%-9s %-9s %s", "22/tcp", "open", "ssh")
	for _, test := range []struct{ line, token, sgr string }{
		{scan, "open", ansiGood},
		{auth, "Failed", ansiAlert},
		{auth, "02:14:07", ansiDim},
		{detail("/usr/sbin/sshd: FAILED"), "FAILED", ansiAlert},
		{detail("%-6s %-9s %5s  %s", "PID", "USER", "%CPU", "COMMAND"), "PID", ansiBoldWhite},
	} {
		if got := renderLine(test.line, color); !strings.Contains(got, test.sgr+test.token) {
			t.Errorf("%q: %q not shaded %q in %q", test.line, test.token, test.sgr, got)
		}
	}
	// Addresses, hosts, ports, paths, and serials carry no color of their own.
	for _, test := range []struct{ line, token string }{
		{auth, "198.51.100.201"},
		{scan, "22/tcp"},
		{field("subject", "CN=web-01.ember.invalid"), "CN=web-01.ember.invalid"},
		{detail("/usr/sbin/sshd: OK"), "/usr/sbin/sshd:"},
		{okLine("198.51.100.201 blocked  rules 14-15  SIM-7F"), "SIM-7F"},
	} {
		got := renderLine(test.line, color)
		at := strings.Index(got, test.token)
		before := got[:max(at, 0)]
		if at < 0 || strings.HasSuffix(before, "m") && !strings.HasSuffix(before, ansiReset) {
			t.Errorf("%q should be plain in %q", test.token, got)
		}
	}
}

func wordmarkLineFor(row int) string {
	return titleInner(centerText(wordmarkRows()[row], screenWidth-8))
}

func TestUnicodeSnapshot(t *testing.T) {
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	model, _ = model.Trigger()
	var b strings.Builder
	for _, line := range model.transcript {
		b.WriteString(renderLine(line, style{unicode: true}))
		b.WriteByte('\n')
	}
	got := b.String()
	path := filepath.Join("testdata", "header-burst-unicode.golden")
	if os.Getenv("NIGHTSHIFT_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("snapshot mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestPacingVariesByKindOfOutput(t *testing.T) {
	cmd := prompt("trace -n -q 1 198.51.100.201")
	timeout := waitBefore(detail(" 2  192.0.2.9         4.100 ms"), detail(" 3  * * *"), paceFor("", "a"))
	logLine := waitBefore(detail("02:14:07 sshd[4127]: Failed password for root from 198.51.100.201"),
		detail("02:14:08 sshd[4127]: Failed password for root from 198.51.100.201"), &rng{state: 1})
	if timeout < time.Second || logLine > 700*time.Millisecond || waitBefore(cmd, detail(" 1  192.0.2.1  0.4 ms"), &rng{}) == 0 {
		t.Fatalf("timeout %v log %v", timeout, logLine)
	}
	fx := loaderEffect(progressLine("syn sweep"), style{unicode: true}, &rng{state: 7})
	distinct := map[time.Duration]bool{}
	var total time.Duration
	for _, delay := range fx.delays {
		distinct[delay] = true
		total += delay
	}
	if len(distinct) < 4 || total < time.Second {
		t.Fatalf("loader pacing is flat: %d distinct delays, %v total", len(distinct), total)
	}
	if len(fx.frames) != len(fx.delays) {
		t.Fatal("frames and delays drifted")
	}
}
