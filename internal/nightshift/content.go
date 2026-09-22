package nightshift

import (
	"fmt"
	"strings"
)

// phaseCopy is authored simulation text. It never includes operator input.
type phaseCopy struct {
	Name       string
	Code       string
	Objective  string
	Hint       string
	ReadyLabel string
}

var phaseCopies = [...]phaseCopy{
	{
		Name:       "BOOT ATTESTATION",
		Code:       "01",
		Objective:  "Local envelope check.",
		Hint:       "The envelope is idle. A key appends one burst. Enter waits.",
		ReadyLabel: "attested",
	},
	{
		Name:       "ANOMALOUS SIGNAL",
		Code:       "02",
		Objective:  "One bounded synthetic signal.",
		Hint:       "The signal is staged. A key appends one burst before Enter.",
		ReadyLabel: "acquired",
	},
	{
		Name:       "DECOY CORRELATION",
		Code:       "03",
		Objective:  "Decoy card matched on the slate.",
		Hint:       "Correlation is waiting. Append one burst. Enter waits for it.",
		ReadyLabel: "correlated",
	},
	{
		Name:       "SIMULATED CONTAINMENT",
		Code:       "04",
		Objective:  "Paper paths held on the slate.",
		Hint:       "Containment is on paper. Append one burst before Enter.",
		ReadyLabel: "contained",
	},
	{
		Name:       "AFTER-ACTION REPORT",
		Code:       "05",
		Objective:  "Local record sealed in memory.",
		Hint:       "The report is unsealed. Append one burst. Enter can close.",
		ReadyLabel: "reported",
	},
}

const finaleBlackout = "BLACKOUT. the glass is dark. the seed remains."

const (
	screenWidth = 72
	burstMin    = 8
	burstMax    = 16
)

const (
	wordmarkGlyphWidth = 5
	wordmarkWord       = "NIGHTSHIFT"
)

var wordmarkGlyphs = map[byte][5]string{
	'N': {"#   #", "##  #", "# # #", "#  ##", "#   #"},
	'I': {"#####", "  #  ", "  #  ", "  #  ", "#####"},
	'G': {" ####", "#    ", "# ###", "#   #", " ####"},
	'H': {"#   #", "#   #", "#####", "#   #", "#   #"},
	'T': {"#####", "  #  ", "  #  ", "  #  ", "  #  "},
	'S': {"#####", "#    ", "#####", "    #", "#####"},
	'F': {"#####", "#    ", "#### ", "#    ", "#    "},
}

func wordmarkRows() [5]string {
	var rows [5]string
	for r := 0; r < 5; r++ {
		parts := make([]string, len(wordmarkWord))
		for i := 0; i < len(wordmarkWord); i++ {
			parts[i] = wordmarkGlyphs[wordmarkWord[i]][r]
		}
		rows[r] = strings.Join(parts, " ")
	}
	return rows
}

func borderTop(label string) string {
	core := "==[ " + label + " ]"
	pad := screenWidth - len(core) - 2
	if pad < 0 {
		pad = 0
	}
	return "+" + core + strings.Repeat("=", pad) + "+"
}

func borderBottom() string {
	return "+" + strings.Repeat("=", screenWidth-2) + "+"
}

func titleInner(text string) string {
	const inner = screenWidth - 8
	gap := inner - len(text)
	if gap < 0 {
		gap = 0
	}
	return "|== " + text + strings.Repeat(" ", gap) + " ==|"
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	left := (width - len(text)) / 2
	return strings.Repeat(" ", left) + text
}

func headerLines(seed uint64, callsign string) []string {
	lines := []string{
		borderTop("NIGHTSHIFT"),
		titleInner(""),
	}
	rows := wordmarkRows()
	for _, row := range rows {
		lines = append(lines, titleInner(centerText(row, screenWidth-8)))
	}
	lines = append(lines,
		titleInner(centerText(strings.Repeat("=", len(strings.TrimRight(rows[0], " "))), screenWidth-8)),
		titleInner(""),
		titleInner(centerText(fmt.Sprintf("SEED %016X    CALLSIGN %s", seed, callsign), screenWidth-8)),
		titleInner(""),
		borderBottom(),
		"// a key appends one burst. enter hints, then advances. ctrl+c exits.",
		"",
	)
	return lines
}

func commentary(phase Phase) string {
	if phase < 0 || int(phase) >= len(kikiLines) {
		return ""
	}
	return kikiLines[phase]
}

func phaseBanner(phase Phase) []string {
	copy := phaseCopies[phase]
	return []string{
		borderTop(copy.Code + " " + copy.Name),
		titleInner("  " + copy.Objective),
		borderBottom(),
		commentary(phase),
	}
}

func advanceLines(from, to Phase) []string {
	prev := phaseCopies[from]
	lines := []string{
		fmt.Sprintf("[ok] %s %s  %s", prev.Code, prev.Name, prev.ReadyLabel),
		"",
	}
	return append(lines, phaseBanner(to)...)
}

func finaleLines() []string {
	last := phaseCopies[PhaseReport]
	return []string{
		fmt.Sprintf("[ok] %s %s  %s", last.Code, last.Name, last.ReadyLabel),
		"",
		borderTop("MISSION COMPLETE"),
		titleInner("  five phases closed. no external system was contacted."),
		titleInner("  " + finaleBlackout),
		borderBottom(),
		"K.I.K.I. // The glass went dark. The seed is still here.",
		"K.I.K.I. // Enter runs this seed again. I did not keep the keys.",
		"// ENTER restarts the same seeded mission. CTRL+C exits.",
	}
}

func restartLines(seed uint64) []string {
	lines := []string{
		fmt.Sprintf("// RESTART same seed %016X continues", seed),
		"",
	}
	return append(lines, phaseBanner(PhaseBoot)...)
}

// Hosts below end in .invalid. Addresses stay in documentation ranges.
// burstLines reads only the seed and the trigger count.

type site struct {
	tag   string
	host  string
	addr  string
	state string
	bars  string
	note  string
	seal  string
	find  string
}

var sites = []site{
	{tag: "ember", host: "ember.invalid", addr: "192.0.2.44", state: "parked", bars: "####----", note: "synthetic carrier", seal: "SIM-7F", find: "FND-2041"},
	{tag: "orbital", host: "orbital-fable.invalid", addr: "198.51.100.23", state: "matched", bars: "##------", note: "decoy card", seal: "SIM-2C", find: "FND-1880"},
	{tag: "quiet", host: "quiet-room.invalid", addr: "203.0.113.10", state: "shut", bars: "######--", note: "paper path", seal: "SIM-9A", find: "FND-2204"},
	{tag: "lantern", host: "lantern-row.invalid", addr: "192.0.2.10", state: "warm", bars: "###-----", note: "cue light", seal: "SIM-4D", find: "FND-1766"},
	{tag: "dock", host: "paper-dock.invalid", addr: "198.51.100.7", state: "copied", bars: "#####---", note: "memory manifest", seal: "SIM-1B", find: "FND-2310"},
	{tag: "loft", host: "cue-loft.invalid", addr: "203.0.113.4", state: "marked", bars: "#-------", note: "rehearsal mark", seal: "SIM-6E", find: "FND-1599"},
	{tag: "gallery", host: "moth-gallery.invalid", addr: "192.0.2.80", state: "faded", bars: "#######-", note: "prop signal", seal: "SIM-8C", find: "FND-2402"},
	{tag: "annex", host: "seal-annex.invalid", addr: "198.51.100.90", state: "holding", bars: "####----", note: "pencil check", seal: "SIM-3A", find: "FND-1420"},
	{tag: "index", host: "night-index.invalid", addr: "203.0.113.55", state: "indexed", bars: "##------", note: "slate index", seal: "SIM-5F", find: "FND-2677"},
	{tag: "yard", host: "fable-yard.invalid", addr: "192.0.2.18", state: "counted", bars: "######--", note: "yard card", seal: "SIM-7A", find: "FND-1904"},
}

func (s site) fieldLines() []string {
	return []string{
		fmt.Sprintf("  host: %s", s.host),
		fmt.Sprintf("  addr: %s", s.addr),
		fmt.Sprintf("  state: %s", s.state),
		fmt.Sprintf("  note: %s", s.note),
		fmt.Sprintf("  seal: %s", s.seal),
		fmt.Sprintf("  find: %s", s.find),
		fmt.Sprintf("  latch: %s shut", s.tag),
		fmt.Sprintf("  noise: %s hush", s.tag),
	}
}

func (s site) okLine() string {
	return fmt.Sprintf("[ok] %s %s", s.host, s.state)
}

func (s site) warnLine() string {
	return fmt.Sprintf("[warn] %s stays on the card", s.host)
}

func (s site) commentLine() string {
	return fmt.Sprintf("// %s noted on the glass", s.host)
}

func (s site) meterLine() string {
	return fmt.Sprintf("  %-8s [%s]", s.tag, s.bars)
}

func (s site) box() [3]string {
	const inner = 28
	core := fmt.Sprintf("+--[ %s ]", s.tag)
	dash := inner - len(core) - 1
	if dash < 1 {
		dash = 1
	}
	top := "  " + core + strings.Repeat("-", dash) + "+"
	inside := inner - 2
	body := fmt.Sprintf("%s [%s]", s.state, s.bars)
	if len(body) > inside {
		body = body[:inside]
	}
	gap := inside - len(body)
	left := 1
	if gap < 1 {
		left = 0
	}
	right := gap - left
	if right < 0 {
		right = 0
	}
	mid := "  |" + strings.Repeat(" ", left) + body + strings.Repeat(" ", right) + "|"
	return [3]string{top, mid, top}
}

func (s site) detailLines(cursor, count int) []string {
	pool := s.fieldLines()
	out := make([]string, 0, count)
	if count >= 3 {
		take := count - 1
		if take < 2 {
			take = 2
		}
		out = append(out, pool[0], pool[1])
		rest := pool[2:]
		need := take - 2
		if need > 0 {
			start := cursor % len(rest)
			for i := 0; i < need; i++ {
				out = append(out, rest[(start+i)%len(rest)])
			}
		}
		out = append(out, s.warnLine())
		return out
	}
	for i := 0; i < count && i < len(pool); i++ {
		out = append(out, pool[i])
	}
	return out
}

var kikiLines = []string{
	"K.I.K.I. // The line went green. Leave it.",
	"K.I.K.I. // I am reading it with you.",
	"K.I.K.I. // That meter is a drawing.",
	"K.I.K.I. // It does not open anything.",
	"K.I.K.I. // Local note. I checked.",
	"K.I.K.I. // I would have said if it were not.",
	"K.I.K.I. // The seed matches. Do not get soft.",
	"K.I.K.I. // Sentiment is not a status.",
	"K.I.K.I. // Do not romanticize a prompt.",
	"K.I.K.I. // It finished. That is all.",
	"K.I.K.I. // I did not keep what you typed.",
	"K.I.K.I. // The letters are already gone.",
	"K.I.K.I. // Scroll up. The earlier note is there.",
	"K.I.K.I. // I am not going to repeat it.",
	"K.I.K.I. // Enter moves the phase.",
	"K.I.K.I. // This key does not.",
	"K.I.K.I. // Yellow is a lamp.",
	"K.I.K.I. // Green is the note.",
	"K.I.K.I. // Host names are costumes.",
	"K.I.K.I. // I do not visit them.",
	"K.I.K.I. // The box is status.",
	"K.I.K.I. // Not a way out.",
	"K.I.K.I. // Quiet means the slate is quiet.",
	"K.I.K.I. // It is not a place.",
	"K.I.K.I. // Callsign is printed.",
	"K.I.K.I. // It still does nothing.",
	"K.I.K.I. // FND-2041 is a serial for a fiction.",
	"K.I.K.I. // Remember that, then let it go.",
	"K.I.K.I. // AAR-19 lives until you exit.",
	"K.I.K.I. // Then I drop it.",
	"K.I.K.I. // Nothing in this burst leaves the terminal.",
	"K.I.K.I. // I am still on the glass.",
}

var prompts []string
var corpus []string

func init() {
	prompts = buildPrompts()
	corpus = buildCorpus()
}

func buildPrompts() []string {
	generic := []string{
		"nightshift> attest --local",
		"nightshift> status --quiet",
		"nightshift> meter --glass",
		"nightshift> seal --local",
		"nightshift> index --show",
		"nightshift> mark --cue",
		"nightshift> ledger --local",
		"nightshift> hold --paper",
		"nightshift> align --cards",
		"nightshift> file --memory",
	}
	out := make([]string, 0, len(sites)+len(generic))
	for _, item := range sites {
		out = append(out, "nightshift> note "+item.host)
	}
	return append(out, generic...)
}

func buildCorpus() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 80)
	add := func(line string) {
		if line == "" {
			panic("empty corpus line")
		}
		if len(line) > screenWidth {
			panic("corpus line too wide: " + line)
		}
		for _, r := range line {
			if r > 127 || r == '·' {
				panic("corpus line not ascii")
			}
		}
		if _, ok := seen[line]; ok {
			panic("duplicate corpus line: " + line)
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	for _, line := range prompts {
		add(line)
	}
	for _, item := range sites {
		for _, line := range item.fieldLines() {
			add(line)
		}
		add(item.okLine())
		add(item.warnLine())
		add(item.commentLine())
		add(item.meterLine())
		box := item.box()
		add(box[0])
		add(box[1])
		if box[2] != box[0] {
			add(box[2])
		}
	}
	for _, line := range kikiLines {
		add(line)
	}
	if len(out) < 80 {
		panic("corpus shorter than 80")
	}
	return out
}

func burstLen(seed, index uint64) int {
	mixed := splitmix(seed ^ ((index + 1) * 0x9e3779b97f4a7c15))
	span := burstMax - burstMin + 1
	return burstMin + int(mixed%uint64(span))
}

func burstOrigin(seed, trigger uint64) int {
	cursor := int(splitmix(seed) % 4096)
	for i := uint64(0); i < trigger; i++ {
		cursor += burstLen(seed, i)
	}
	return cursor
}

// burstLines is the burst for this seed and zero-based trigger count.
// Consecutive counts move the cursor forward, so they do not repeat.
func burstLines(seed, trigger uint64) []string {
	n := burstLen(seed, trigger)
	cursor := burstOrigin(seed, trigger)
	item := sites[cursor%len(sites)]
	deco := 1
	if n >= 11 {
		deco = 3
	}
	fieldCount := n - 5 - deco
	lines := make([]string, 0, n)
	lines = append(lines, prompts[cursor%len(prompts)])
	lines = append(lines, item.okLine())
	lines = append(lines, item.detailLines(cursor, fieldCount)...)
	lines = append(lines, item.commentLine())
	if deco == 3 {
		box := item.box()
		lines = append(lines, box[0], box[1], box[2])
	} else {
		lines = append(lines, item.meterLine())
	}
	lines = append(lines,
		kikiLines[cursor%len(kikiLines)],
		kikiLines[(cursor+1)%len(kikiLines)],
	)
	return lines
}
