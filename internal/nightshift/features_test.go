package nightshift

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAgentSwarmBurstIsDeterministicAndLocal(t *testing.T) {
	left := strings.Join(agentSwarm(newBurst(42, 3, PhaseCorrelation)), "\n")
	right := strings.Join(agentSwarm(newBurst(42, 3, PhaseCorrelation)), "\n")
	if left != right {
		t.Fatal("same seed produced different agent swarm output")
	}
	for _, want := range []string{"agent swarm spawn", "orchestrator", "scout", "forensics", "network", "policy", "synthesizer", "local-only", "no external actions"} {
		if !strings.Contains(left, want) {
			t.Fatalf("agent swarm omitted %q: %s", want, left)
		}
	}
}

func TestThreatAndSignalWaveformFollowPhase(t *testing.T) {
	boot := strings.Join(phaseBanner(PhaseBoot), "\n")
	signal := strings.Join(phaseBanner(PhaseSignal), "\n")
	containment := strings.Join(phaseBanner(PhaseContainment), "\n")
	report := strings.Join(phaseBanner(PhaseReport), "\n")
	if !strings.Contains(boot, "THREATCON #---- QUIET") || !strings.Contains(signal, "THREATCON ##--- ELEVATED") ||
		!strings.Contains(containment, "THREATCON ##### CRITICAL") || !strings.Contains(report, "THREATCON #---- CONTAINED") {
		t.Fatalf("phase threat levels are wrong: boot=%q signal=%q containment=%q report=%q", boot, signal, containment, report)
	}
	if strings.Index(signal, "THREATCON") > strings.Index(signal, waveformLine) || !strings.Contains(signal, waveformLine) {
		t.Fatalf("waveform is not under the phase 02 threat line: %q", signal)
	}
	settled := waveformLineText(style{unicode: true})
	if !strings.Contains(settled, "▂▁▂▇▂▁▂▇") {
		t.Fatalf("settled waveform is not beacon-like: %q", settled)
	}
	fx := waveformEffect(style{unicode: true})
	if len(fx.frames) != 4 || !strings.Contains(fx.frames[len(fx.frames)-1], "▂▁▂▇▂▁▂▇") {
		t.Fatalf("waveform animation does not settle: %#v", fx.frames)
	}
}

func TestTracerouteBuildsDeterministicMapAndLatencyLegs(t *testing.T) {
	build := func() []string {
		return traceRoute(&burst{r: &rng{state: 918}, remote: "198.51.100.201"})
	}
	lines := build()
	if strings.Join(lines, "\n") != strings.Join(build(), "\n") {
		t.Fatal("same route seed produced different map data")
	}
	var directive string
	for _, line := range lines {
		if strings.HasPrefix(line, traceMapPrefix) {
			directive = line
		}
	}
	hops := parseTraceMap(directive)
	if len(hops) < 4 {
		t.Fatalf("map route has too few cities: %q", directive)
	}
	for _, hop := range hops {
		if !strings.Contains(strings.Join(lines, "\n"), hop.city) || hop.latencyMs < 2 {
			t.Fatalf("hop %q is missing its city or latency", hop.city)
		}
	}
	rows := strings.Join(traceMapRows(hops, len(hops)), "\n")
	if !strings.Contains(rows, "SIMULATED ROUTE") || !strings.Contains(rows, hops[0].city) || !strings.Contains(rows, "LEG ") {
		t.Fatalf("route map omitted its map, city, or live leg: %q", rows)
	}
	if strings.Contains(rows, "*LHR") || !strings.Contains(rows, "ROUTE  1 "+hops[0].city) {
		t.Fatalf("route map should use numbered pins and a separate legend: %q", rows)
	}
	for _, row := range traceMapRows(hops, len(hops)) {
		if len(row) > screenWidth {
			t.Fatalf("route map row exceeds %d columns: %q", screenWidth, row)
		}
	}
	var output bytes.Buffer
	if err := renderLines(&output, []string{directive}, style{motion: false}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), traceMapPrefix) || !strings.Contains(output.String(), "ASIA-PACIFIC") {
		t.Fatalf("map directive was not rendered as the ASCII map: %q", output.String())
	}
	previousSleep := sleep
	sleep = func(time.Duration) {}
	t.Cleanup(func() { sleep = previousSleep })
	output.Reset()
	if err := renderLines(&output, []string{directive}, style{motion: true}); err != nil {
		t.Fatal(err)
	}
	repaint := fmt.Sprintf("\x1b[%dA", len(traceMapRows(hops, len(hops))))
	if got := strings.Count(output.String(), repaint); got != len(hops)-1 {
		t.Fatalf("route map advanced %d times for %d hops", got, len(hops))
	}
}

func TestTraceMapRevealsNumberedPinsWithoutOverlayingCityNames(t *testing.T) {
	hops := []traceHop{{city: "JFK", latencyMs: 25}, {city: "LHR", latencyMs: 110}, {city: "FRA", latencyMs: 42}}
	first := strings.Join(traceMapRows(hops, 1), "\n")
	final := strings.Join(traceMapRows(hops, len(hops)), "\n")
	if strings.Contains(first, "2 LHR") || !strings.Contains(first, "ROUTE  1 JFK") {
		t.Fatalf("first frame revealed future hops: %q", first)
	}
	if !strings.Contains(final, "ROUTE  1 JFK  >  2 LHR  >  3 FRA") {
		t.Fatalf("final route legend is unclear: %q", final)
	}
	for _, city := range []string{"*JFK", "*LHR", "*FRA"} {
		if strings.Contains(final, city) {
			t.Fatalf("city label %q still overlays the map: %q", city, final)
		}
	}
}

func TestMapDirectiveValidationIsStrictAndNeverLeaks(t *testing.T) {
	valid := traceMapPrefix + "IAD:12,FRA:2400"
	if got := parseTraceMap(valid); len(got) != 2 || got[1].latencyMs != 2400 {
		t.Fatalf("valid route rejected: %#v", got)
	}
	tooMany := make([]string, maxTraceHops+1)
	for i := range tooMany {
		tooMany[i] = "IAD:10"
	}
	bad := []string{
		traceMapPrefix + "IAD:10suffix,FRA:20",
		traceMapPrefix + "IAD:10:extra,FRA:20",
		traceMapPrefix + "IAD:0,FRA:20",
		traceMapPrefix + "IAD:2501,FRA:20",
		traceMapPrefix + "IAD:-1,FRA:20",
		traceMapPrefix + "IAD:10,XXX:20",
		traceMapPrefix + strings.Join(tooMany, ","),
	}
	for _, directive := range bad {
		if got := parseTraceMap(directive); got != nil {
			t.Fatalf("malformed route accepted: %q => %#v", directive, got)
		}
		var output bytes.Buffer
		if err := renderLines(&output, []string{directive}, style{}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.String(), traceMapPrefix) {
			t.Fatalf("internal map directive leaked to terminal: %q", output.String())
		}
	}
}

func TestNarrowTerminalUsesStaticFrames(t *testing.T) {
	if terminalRepaintFits(screenWidth-1) || !terminalRepaintFits(screenWidth) {
		t.Fatal("72-column repaint boundary is wrong")
	}
	hops := parseTraceMap(traceMapPrefix + "IAD:12,FRA:80,SIN:190")
	var output bytes.Buffer
	if err := renderLines(&output, []string{traceMapDirective(hops)}, style{motion: terminalRepaintFits(screenWidth - 1)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") || !strings.Contains(output.String(), "SIMULATED ROUTE") {
		t.Fatalf("narrow terminal did not receive a static map: %q", output.String())
	}
	output.Reset()
	if err := drawDashboard(&output, dashboardLines(NewModel(Config{Seed: 1, SeedProvided: true})), style{}, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("static dashboard used cursor controls: %q", output.String())
	}
}

func TestRadioChatterIsOccasionalAndSeeded(t *testing.T) {
	seen := 0
	for seed := uint64(0); seed < 64; seed++ {
		left := radioChatter(seed, seed%7, PhaseSignal)
		right := radioChatter(seed, seed%7, PhaseSignal)
		if left != right {
			t.Fatalf("radio chatter diverged for seed %d", seed)
		}
		if left != "" {
			seen++
			if !strings.HasPrefix(left, radioPrefix) {
				t.Fatalf("unexpected chatter line %q", left)
			}
		}
	}
	if seen == 0 || seen == 64 {
		t.Fatalf("chatter should be occasional, saw it %d times", seen)
	}
}

func TestDashboardRepaintsFixedLinesAndChangesOnTicks(t *testing.T) {
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	first := dashboardLines(model)
	model = model.Tick(model.started.Add(tickEvery))
	second := dashboardLines(model)
	if strings.Join(first, "\n") == strings.Join(second, "\n") {
		t.Fatal("telemetry did not change on a timer tick")
	}
	joined := strings.Join(second, "\n")
	for _, label := range []string{"CPU", "NET IN", "NET OUT", "ALERTS"} {
		if !strings.Contains(joined, label) {
			t.Fatalf("dashboard omitted %s: %q", label, joined)
		}
	}
	var output bytes.Buffer
	if err := drawDashboard(&output, first, style{}, false); err != nil {
		t.Fatal(err)
	}
	if err := drawDashboard(&output, second, style{}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[4A\r") || !strings.Contains(output.String(), "\x1b[2K") {
		t.Fatalf("dashboard did not redraw its fixed rows: %q", output.String())
	}
}

func TestDebriefCountsRecordedShiftActivity(t *testing.T) {
	current := time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)
	previous := Now
	Now = func() time.Time { return current }
	t.Cleanup(func() { Now = previous })
	model := NewModel(Config{Seed: 42, SeedProvided: true})
	for phase := PhaseBoot; phase <= PhaseReport; phase++ {
		model, _ = model.Trigger()
		if phase != PhaseReport {
			model, _ = model.Enter()
		}
	}
	current = model.started.Add(97 * time.Second)
	model, lines := model.Enter()
	joined := strings.Join(lines, "\n")
	stats := model.missionStats()
	if !model.finale || stats.commands < 5 || stats.hosts < 1 || stats.elapsed != 97*time.Second || stats.grade != "CLEAN HANDOFF" {
		t.Fatalf("bad final shift stats: %#v", stats)
	}
	for _, value := range []string{"SHIFT DEBRIEF", "COMMANDS RUN", "HOSTS TOUCHED", "ALERTS", "TIME ON SHIFT  01:37", "CLEAN HANDOFF"} {
		if !strings.Contains(joined, value) {
			t.Fatalf("debrief omitted %q: %q", value, joined)
		}
	}
}

func TestBellRingsOnlyForWarningLines(t *testing.T) {
	lines := []string{okLine("ready"), warnLine("simulated alert")}
	var silent, bell bytes.Buffer
	if err := renderLines(&silent, lines, style{}); err != nil {
		t.Fatal(err)
	}
	if err := renderLines(&bell, lines, style{bell: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(silent.String(), "\a") || strings.Count(bell.String(), "\a") != 1 {
		t.Fatalf("bell behavior was wrong: silent=%q bell=%q", silent.String(), bell.String())
	}
}

func TestGlitchEffectIsSeededAndSettles(t *testing.T) {
	previousSleep := sleep
	sleep = func(time.Duration) {}
	t.Cleanup(func() { sleep = previousSleep })
	line := "       checksum verification complete"
	for index := 0; index < 10000 && !shouldGlitch("", line); index++ {
		line = fmt.Sprintf("       checksum verification %04d", index)
	}
	if !shouldGlitch("", line) {
		t.Fatal("could not find a deterministic glitch sample")
	}
	st := style{motion: true}
	left, right := effectFor("", line, st), effectFor("", line, st)
	if strings.Join(left.frames, "\n") != strings.Join(right.frames, "\n") || len(left.frames) < 7 {
		t.Fatalf("glitch frames are not deterministic: %d", len(left.frames))
	}
	var output bytes.Buffer
	if err := renderLines(&output, []string{line}, st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\r"+line+"\x1b[K\r\n") {
		t.Fatalf("glitch failed to settle to source text: %q", output.String())
	}
}
