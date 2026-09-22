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
		Objective:  "Verify toolchain, keys, and host integrity.",
		Hint:       "The envelope is idle. A key appends one burst. Enter waits.",
		ReadyLabel: "attested",
	},
	{
		Name:       "ANOMALOUS SIGNAL",
		Code:       "02",
		Objective:  "Unscheduled egress on a watched segment.",
		Hint:       "The signal is staged. A key appends one burst before Enter.",
		ReadyLabel: "acquired",
	},
	{
		Name:       "DECOY CORRELATION",
		Code:       "03",
		Objective:  "Match the beacon against honeypot logs.",
		Hint:       "Correlation is waiting. Append one burst. Enter waits for it.",
		ReadyLabel: "correlated",
	},
	{
		Name:       "SIMULATED CONTAINMENT",
		Code:       "04",
		Objective:  "Isolate the host. Keep the evidence intact.",
		Hint:       "Containment is on paper. Append one burst before Enter.",
		ReadyLabel: "contained",
	},
	{
		Name:       "AFTER-ACTION REPORT",
		Code:       "05",
		Objective:  "Seal the findings and hand off.",
		Hint:       "The report is unsealed. Append one burst. Enter can close.",
		ReadyLabel: "reported",
	},
}

const finaleBlackout = "BLACKOUT. the glass is dark. the seed remains."

const waveformLine = "~ BEACON SPECTRUM"

const (
	screenWidth = 72
	burstMin    = 4
	burstMax    = 13
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

// bootLines follow the header once per session. Each loader is a drawing.
func bootLines() []string {
	lines := make([]string, 0, len(bootLabels)+2)
	for _, label := range bootLabels {
		lines = append(lines, progressLine(label))
	}
	return append(lines, "// uplink armed. nothing leaves this terminal.", "")
}

var bootLabels = [...]string{"kernel link", "entropy pool", "cipher suite", "ghost relay"}

func phaseBanner(phase Phase) []string {
	copy := phaseCopies[phase]
	lines := []string{
		borderTop(copy.Code + " " + copy.Name),
		titleInner("  " + copy.Objective),
		borderBottom(),
		threatLine(phase),
	}
	if phase == PhaseSignal {
		lines = append(lines, waveformLine)
	}
	return lines
}

func advanceLines(from, to Phase) []string {
	prev := phaseCopies[from]
	lines := []string{
		"",
		fmt.Sprintf(okPrefix+"%s %s  %s", prev.Code, prev.Name, prev.ReadyLabel),
		"",
	}
	return append(lines, phaseBanner(to)...)
}

func finaleLines(stats ...missionStats) []string {
	last := phaseCopies[PhaseReport]
	lines := []string{
		"",
		fmt.Sprintf(okPrefix+"%s %s  %s", last.Code, last.Name, last.ReadyLabel),
		"",
		borderTop("MISSION COMPLETE"),
		titleInner("  five phases closed. no external system was contacted."),
		borderBottom(),
	}
	if len(stats) > 0 {
		lines = append(lines, debriefLines(stats[0])...)
	}
	lines = append(lines,
		"",
		titleInner("  "+finaleBlackout),
		"// ENTER restarts the same seeded mission. CTRL+C exits.",
	)
	return lines
}

func restartLines(seed uint64) []string {
	lines := []string{
		fmt.Sprintf("// RESTART same seed %016X continues", seed),
		"",
	}
	return append(lines, phaseBanner(PhaseBoot)...)
}
