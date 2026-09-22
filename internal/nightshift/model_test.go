package nightshift

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func readyThrough(model Model, phase Phase) Model {
	for model.phase != phase || !model.ready {
		if model.finale {
			break
		}
		if !model.ready {
			model, _ = model.Trigger()
			continue
		}
		model, _ = model.Enter()
	}
	return model
}

func TestTriggersAppendBurstsWithoutRetainingInput(t *testing.T) {
	secret := "SUPERSECRETINPUT"
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})
	if _, err = writer.Write([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	parser := &byteReader{f: reader}
	var returned []string
	triggers := 0
	var first, second string
	for {
		kind, readErr := parser.nextKey()
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				t.Fatal(readErr)
			}
			break
		}
		if kind != keyTrigger {
			t.Fatalf("kind %d", kind)
		}
		triggers++
		before := len(model.transcript)
		var lines []string
		var quit bool
		model, lines, quit = model.Handle(kind)
		if quit || !model.ready || model.phase != PhaseBoot || model.triggerCount != uint64(triggers) {
			t.Fatalf("burst %d quit %v ready %v phase %d count %d", triggers, quit, model.ready, model.phase, model.triggerCount)
		}
		if !validBurst(lines) {
			t.Fatalf("burst %d lines %d", triggers, len(lines))
		}
		if len(model.transcript) != before+len(lines) || strings.Join(model.transcript[before:], "\n") != strings.Join(lines, "\n") {
			t.Fatal("trigger did not append only the new burst")
		}
		joined := strings.Join(lines, "\n")
		switch triggers {
		case 1:
			first = joined
		case 2:
			second = joined
		}
		returned = append(returned, lines...)
	}
	if triggers != len(secret) {
		t.Fatalf("keyTrigger results %d, bytes %d", triggers, len(secret))
	}
	if second == "" || second == first || model.phase != PhaseBoot || model.triggerCount != uint64(len(secret)) {
		t.Fatalf("same-phase burst repeated or advanced: phase %d count %d", model.phase, model.triggerCount)
	}
	var lines []string
	var quit bool
	model, lines, quit = model.Handle(keyIgnore)
	if quit || lines != nil || model.triggerCount != uint64(len(secret)) {
		t.Fatal("ignored input changed the transcript")
	}
	if strings.Contains(model.Transcript(), secret) || strings.Contains(model.Transcript(), "SUPER") {
		t.Fatal("transcript retained input")
	}
	if joined := strings.Join(returned, "\n"); strings.Contains(joined, secret) || strings.Contains(joined, "SUPER") {
		t.Fatal("returned lines retained input")
	}
	if dump := fmt.Sprintf("%#v", model); strings.Contains(dump, secret) || strings.Contains(dump, "SUPER") {
		t.Fatal("model dump retained input")
	}
}

func TestEarlyEnterHintsAndReadyAdvancesEveryPhase(t *testing.T) {
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	if strings.Contains(model.Transcript(), "LOCK ") || !strings.Contains(model.Transcript(), "01 BOOT ATTESTATION") || !strings.Contains(model.Transcript(), phaseCopies[0].Objective) {
		t.Fatal(model.Transcript())
	}
	var lines []string
	model, lines = model.Enter()
	if model.ready || model.phase != PhaseBoot || model.triggerCount != 0 || model.lastHint != phaseCopies[0].Hint {
		t.Fatalf("early enter = %#v", model)
	}
	if len(lines) != 1 || lines[0] != "// "+phaseCopies[0].Hint {
		t.Fatalf("hint = %#v", lines)
	}
	model, _ = model.Enter()
	if model.phase != PhaseBoot || model.ready || model.triggerCount != 0 {
		t.Fatal("repeated early enter changed the mission")
	}
	hinted := NewModel(Config{Seed: 42, SeedProvided: true})
	hinted, _ = hinted.Enter()
	hinted, lines = hinted.Trigger()
	if !validBurst(lines) || strings.Join(lines[1:], "\n") != strings.Join(burstLines(42, 0, PhaseBoot), "\n") {
		t.Fatal("hint consumed a trigger")
	}

	for phase := PhaseBoot; phase <= PhaseReport; phase++ {
		model = readyThrough(model, phase)
		if !model.ready || model.phase != phase {
			t.Fatalf("phase %d model %#v", phase, model)
		}
		if phase == PhaseReport {
			break
		}
		var advanced []string
		model, advanced = model.Enter()
		if model.phase != phase+1 || model.ready || model.lastHint != "" {
			t.Fatalf("advance = %#v", model)
		}
		banner := strings.Join(advanced, "\n")
		if !strings.Contains(banner, phaseCopies[phase+1].Name) || strings.Contains(banner, "K.I.K.I") {
			t.Fatalf("banner = %s", banner)
		}
	}
	before := model.triggerCount
	var finale []string
	model, finale = model.Enter()
	if !model.finale || model.phase != PhaseReport || model.triggerCount != before {
		t.Fatalf("finale = %#v", model)
	}
	if !strings.Contains(strings.Join(finale, "\n"), finaleBlackout) {
		t.Fatal(finale)
	}
	quiet, extra := model.Trigger()
	if !quiet.finale || extra != nil || quiet.Transcript() != model.Transcript() {
		t.Fatal("trigger during finale appended text")
	}
}

func TestFinaleEnterRestartsSameSeed(t *testing.T) {
	model := NewModel(Config{Seed: 99, SeedProvided: true, ASCII: true, NoColor: true, ReducedMotion: true})
	model = readyThrough(model, PhaseReport)
	model, _ = model.Enter()
	if !model.finale {
		t.Fatal("expected finale")
	}
	finaleView := model.Transcript()
	if !strings.Contains(finaleView, finaleBlackout) || !strings.Contains(finaleView, "CTRL+C exits") {
		t.Fatal(finaleView)
	}
	if strings.Count(finaleView, wordmarkRows()[0]) != 1 {
		t.Fatal("header repeated before restart")
	}
	before := len(model.transcript)
	restarted, lines := model.Enter()
	if restarted.finale || restarted.ready || restarted.phase != PhaseBoot || restarted.triggerCount != 0 {
		t.Fatalf("restart = %#v", restarted)
	}
	if restarted.config != model.config || restarted.cosmetic != model.cosmetic {
		t.Fatalf("seed/config = %#v cosmetic %#v", restarted.config, restarted.cosmetic)
	}
	if len(restarted.transcript) <= before {
		t.Fatal("restart cleared the transcript")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "RESTART same seed 0000000000000063 continues") || !strings.Contains(joined, phaseCopies[0].Name) {
		t.Fatal(joined)
	}
	if strings.Count(restarted.Transcript(), wordmarkRows()[0]) != 1 {
		t.Fatal("restart reprinted the header")
	}
	if !strings.Contains(restarted.Transcript(), "MISSION COMPLETE") {
		t.Fatal("restart dropped earlier finale lines")
	}
	fresh, freshLines := NewModel(model.config).Trigger()
	_, againLines := restarted.Trigger()
	if strings.Join(freshLines, "\n") != strings.Join(againLines, "\n") {
		t.Fatal("restarted seed did not replay the first burst")
	}
	_ = fresh
}

func TestIgnoredControlsAndPastedInterruptDoNotQuit(t *testing.T) {
	model := NewModel(Config{Seed: 5, SeedProvided: true})
	before := model.Transcript()
	next, lines, quit := model.Handle(keyIgnore)
	if quit || lines != nil || next.Transcript() != before || next.phase != model.phase || next.triggerCount != 0 {
		t.Fatal("ignored control changed state")
	}
	quitModel, quitLines, quit := model.Handle(keyQuit)
	if !quit || quitLines != nil || quitModel.phase != model.phase || quitModel.Transcript() != before {
		t.Fatal("ctrl+c did not quit cleanly")
	}

	payload := []byte("\x1b[200~SUPERSECRETINPUT\x03\x1b[201~")
	pasteReader, pasteWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pasteReader.Close()
		pasteWriter.Close()
	})
	if _, err = pasteWriter.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = pasteWriter.Close(); err != nil {
		t.Fatal(err)
	}
	parser := &byteReader{f: pasteReader}
	var kinds []keyKind
	for {
		kind, readErr := parser.nextKey()
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				t.Fatal(readErr)
			}
			kinds = append(kinds, keyEOF)
			break
		}
		kinds = append(kinds, kind)
	}
	if len(kinds) != 2 || kinds[0] != keyTrigger || kinds[1] != keyEOF {
		t.Fatalf("kinds = %v", kinds)
	}
	for _, kind := range kinds {
		if kind == keyQuit {
			t.Fatalf("kinds = %v", kinds)
		}
	}
	pasted, lines, quit := model.Handle(kinds[0])
	if quit || pasted.triggerCount != 1 || !validBurst(lines) {
		t.Fatal("paste trigger did not append a burst")
	}
	if len(pasted.transcript) != len(model.transcript)+len(lines) || strings.Join(pasted.transcript[len(model.transcript):], "\n") != strings.Join(lines, "\n") {
		t.Fatal("paste trigger did not append a burst")
	}
	if strings.Contains(pasted.Transcript(), "SUPERSECRETINPUT") {
		t.Fatal("paste bytes were retained")
	}
}

func TestTimerTicksDoNotAdvanceStory(t *testing.T) {
	model := NewModel(Config{Seed: 8, SeedProvided: true})
	model, _ = model.Trigger()
	startPhase, startCount, startReady := model.phase, model.triggerCount, model.ready
	startText := model.Transcript()
	stamp := model.started
	for i := 0; i < 12; i++ {
		model = model.Tick(stamp.Add(time.Duration(i) * tickEvery))
	}
	if model.phase != startPhase || model.triggerCount != startCount || model.ready != startReady || model.finale || model.tickCount != 12 {
		t.Fatalf("tick advanced story: %#v", model)
	}
	if model.Transcript() != startText {
		t.Fatal("tick printed lines")
	}
	if model.elapsed == 0 {
		t.Fatal("tick did not record elapsed time")
	}

	steady := NewModel(Config{Seed: 8, SeedProvided: true, ReducedMotion: true})
	moved := steady
	moved.tickCount = 4
	moved.elapsed = 5 * time.Second
	if steady.Transcript() != moved.Transcript() {
		t.Fatal("tick counter changed the transcript")
	}
	animated := NewModel(Config{Seed: 8, SeedProvided: true})
	pulsed := animated
	pulsed.tickCount = 1
	if animated.Transcript() != pulsed.Transcript() {
		t.Fatal("tick counter changed the transcript")
	}
}

func TestSeededCosmeticsAreDeterministic(t *testing.T) {
	left := NewModel(Config{Seed: 42, SeedProvided: true})
	right := NewModel(Config{Seed: 42, SeedProvided: true})
	left = readyThrough(left, PhaseCorrelation)
	right = readyThrough(right, PhaseCorrelation)
	if left.Transcript() != right.Transcript() || left.cosmetic != right.cosmetic {
		t.Fatal("same seed diverged")
	}
	base := cosmeticFor(1).Callsign
	different := false
	for seed := uint64(2); seed < 30; seed++ {
		if cosmeticFor(seed).Callsign != base {
			different = true
			break
		}
	}
	if !different {
		t.Fatal("cosmetics did not vary")
	}
}

// validBurst reports whether appended lines are one blank separator and a burst.
func validBurst(lines []string) bool {
	return len(lines) > 0 && lines[0] == "" && len(lines)-1 >= burstMin && len(lines)-1 <= burstMax
}

func TestFullRunHasNoCommentator(t *testing.T) {
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	for round := 0; round < 2; round++ {
		for phase := PhaseBoot; phase <= PhaseReport; phase++ {
			model, _ = model.Trigger()
			model, _ = model.Trigger()
			model, _ = model.Enter()
		}
		if !model.finale {
			t.Fatal("run did not reach the finale")
		}
		model, _ = model.Enter()
	}
	if strings.Contains(model.Transcript(), "K.I.K.I") || strings.Contains(strings.ToLower(model.Transcript()), "kiki") {
		t.Fatal("commentator line returned")
	}
}

func TestTranscriptStaysAppendOnly(t *testing.T) {
	model := readyThrough(NewModel(Config{Seed: 42, SeedProvided: true}), PhaseCorrelation)
	reject := func(view string) {
		t.Helper()
		if strings.Contains(view, "LOCAL SIMULATION · OFFLINE") || strings.Contains(view, "LOCAL SIMULATION . OFFLINE") {
			t.Fatal(view)
		}
		if strings.Contains(view, "RESIZE PROMPT") || strings.Contains(view, "MISSION TIMELINE") || strings.Contains(view, "<##>") {
			t.Fatal(view)
		}
		if strings.Contains(view, "·") {
			t.Fatal("middle dot returned")
		}
	}
	reject(model.Transcript())
	if !strings.Contains(model.Transcript(), "SEED 000000000000002A") || !strings.Contains(model.Transcript(), "CALLSIGN "+cosmeticFor(42).Callsign) {
		t.Fatal(model.Transcript())
	}
	plain := NewModel(Config{Seed: 42, SeedProvided: true, ASCII: true, NoColor: true, ReducedMotion: true})
	plain, plainLines := plain.Trigger()
	base := NewModel(Config{Seed: 42, SeedProvided: true})
	base, baseLines := base.Trigger()
	if plain.Transcript() != base.Transcript() || strings.Join(plainLines, "\n") != strings.Join(baseLines, "\n") {
		t.Fatal("presentation flags changed the transcript")
	}
	reject(plain.Transcript())

	var buf bytes.Buffer
	fresh := NewModel(Config{Seed: 42, SeedProvided: true})
	if err := writeLines(&buf, fresh.transcript); err != nil {
		t.Fatal(err)
	}
	var burst []string
	fresh, burst = fresh.Trigger()
	if err := writeLines(&buf, burst); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatal("writer emitted a cursor sequence")
	}
	color := style{color: true}
	prompt := renderLine("nightshift> attest --local", color)
	ok := renderLine(okPrefix+"ember.invalid  parked", color)
	if !strings.Contains(prompt, ansiPrompt+"nightshift>") || !strings.Contains(ok, ansiBrightGreen+"[ok]") {
		t.Fatal("color painter missed the prompt or the ok tag")
	}
	if strings.Contains(renderLine("", style{color: true, unicode: true}), "\x1b") {
		t.Fatal("empty line was painted")
	}
	if strings.ReplaceAll(buf.String(), "\r\n", "\n") != fresh.Transcript()+"\n" {
		t.Fatal("writer did not append the same lines the model stored")
	}
}

// sampleBursts is every phase's bursts for a spread of seeds.
func sampleBursts() [][]string {
	var out [][]string
	for _, seed := range []uint64{0, 1, 42, 99, 1 << 32, 0xdeadbeef} {
		for phase := PhaseBoot; phase <= PhaseReport; phase++ {
			for index := uint64(0); index < 12; index++ {
				out = append(out, burstLines(seed, index, phase))
			}
		}
	}
	return out
}

func TestBurstSelectionIsSeedAndCount(t *testing.T) {
	for _, lines := range sampleBursts() {
		if len(lines) < burstMin || len(lines) > burstMax {
			t.Fatalf("burst length %d: %q", len(lines), lines)
		}
		if !strings.HasPrefix(lines[0], promptPrefix) {
			t.Fatalf("burst does not open with a prompt: %q", lines[0])
		}
		for _, line := range lines {
			if line == "" || len(line) > screenWidth || strings.HasSuffix(line, " ") {
				t.Fatalf("burst line shape %q", line)
			}
			for _, r := range line {
				if r > 127 {
					t.Fatalf("non-ascii burst line %q", line)
				}
			}
			known := false
			for _, prefix := range []string{promptPrefix, okPrefix, warnPrefix, progressPrefix, fieldIndent} {
				known = known || strings.HasPrefix(line, prefix)
			}
			if !known {
				t.Fatalf("unexpected shape %q", line)
			}
		}
	}
	for _, seed := range []uint64{0, 1, 42, 99, 1 << 32} {
		for phase := PhaseBoot; phase <= PhaseReport; phase++ {
			commands := map[string]bool{}
			previous := ""
			for index := uint64(0); index < 8; index++ {
				lines := burstLines(seed, index, phase)
				got := strings.Join(lines, "\n")
				if got != strings.Join(burstLines(seed, index, phase), "\n") {
					t.Fatal("burst selection was not stable")
				}
				verb := strings.Fields(strings.TrimPrefix(lines[0], promptPrefix))[0]
				if verb == previous {
					t.Fatalf("seed %d phase %d repeated %s at %d", seed, phase, verb, index)
				}
				previous = verb
				commands[verb] = true
			}
			if len(commands) < 3 {
				t.Fatalf("seed %d phase %d has little variety: %v", seed, phase, commands)
			}
		}
	}
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	for index := uint64(0); index < 6; index++ {
		want := strings.Join(append([]string{""}, burstLines(42, index, PhaseBoot)...), "\n")
		var lines []string
		model, lines = model.Trigger()
		if strings.Join(lines, "\n") != want || model.phase != PhaseBoot {
			t.Fatalf("trigger %d did not follow seed, count, and phase", index)
		}
	}
	if strings.Join(burstLines(99, 0, PhaseBoot), "\n") == strings.Join(burstLines(42, 0, PhaseBoot), "\n") {
		t.Fatal("different seeds printed the same first burst")
	}
}

func TestAuthoredTextStaysFictional(t *testing.T) {
	var blob strings.Builder
	for _, lines := range sampleBursts() {
		for _, line := range lines {
			// A dump row's gutter is raw bytes, not prose; it can look dotted.
			if strings.HasPrefix(line, fieldIndent+"0x") {
				continue
			}
			blob.WriteString(line)
			blob.WriteByte('\n')
		}
	}
	for _, copy := range phaseCopies {
		fmt.Fprintf(&blob, "%s\n%s\n%s\n%s\n", copy.Name, copy.Code, copy.Objective, copy.Hint)
	}
	blob.WriteString(finaleBlackout)
	blob.WriteByte('\n')
	for _, line := range append(append(append(headerLines(42, "MOTHLIGHT"), bootLines()...), finaleLines()...), restartLines(42)...) {
		blob.WriteString(line)
		blob.WriteByte('\n')
	}
	text := blob.String()
	for _, banned := range []string{"CVE-", "http://", "https://", "LOCAL SIMULATION", "·"} {
		if strings.Contains(text, banned) {
			t.Fatalf("banned text %s", banned)
		}
	}
	hosts, addresses := 0, 0
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == ';' || r == '(' || r == ')' || r == '|' || r == '"'
	}) {
		token := strings.Trim(field, ".,:'")
		// Paths may hold dotted file names; an address may carry a prefix or port.
		if strings.Contains(token, "/") {
			token = strings.SplitN(token, "/", 2)[0]
			if token == "" {
				continue
			}
		}
		if host, _, ok := strings.Cut(token, ":"); ok && strings.Count(host, ".") == 3 {
			token = host
		}
		if strings.Contains(token, "=") {
			token = token[strings.LastIndex(token, "=")+1:]
		}
		if ip := net.ParseIP(token); ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				if !documentationIPv4(ip4) {
					t.Fatalf("address outside documentation ranges: %s", token)
				}
			} else if !strings.HasPrefix(token, "2001:db8:") {
				t.Fatalf("ipv6 outside documentation range: %s", token)
			}
			addresses++
			continue
		}
		labels := strings.Split(token, ".")
		last := labels[len(labels)-1]
		if len(labels) > 1 && len(last) >= 2 && strings.Trim(last, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") == "" {
			hosts++
			if last != "invalid" {
				t.Fatalf("host %s", token)
			}
		}
	}
	if hosts == 0 || addresses == 0 || !strings.Contains(text, "192.0.2.") || !strings.Contains(text, "198.51.100.") || !strings.Contains(text, "203.0.113.") {
		t.Fatal("expected fictional hosts and documentation addresses")
	}
}

func documentationIPv4(ip net.IP) bool {
	switch {
	case ip[0] == 192 && ip[1] == 0 && ip[2] == 2:
		return true
	case ip[0] == 198 && ip[1] == 51 && ip[2] == 100:
		return true
	case ip[0] == 203 && ip[1] == 0 && ip[2] == 113:
		return true
	default:
		return false
	}
}
