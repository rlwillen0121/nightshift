package nightshift

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	traceMapPrefix    = "// TRACE MAP "
	maxTraceHops      = 12
	maxTraceLatencyMS = 2500
	radioPrefix       = "[radio] "
)

type missionStats struct {
	commands int
	hosts    int
	alerts   int
	elapsed  time.Duration
	grade    string
}

func (m Model) missionStats() missionStats {
	return missionStats{
		commands: m.commandsRun,
		hosts:    len(m.hostsTouched),
		alerts:   m.alertCount,
		elapsed:  m.elapsed,
		grade:    "CLEAN HANDOFF",
	}
}

func debriefLines(stats missionStats) []string {
	minutes := int(stats.elapsed / time.Minute)
	seconds := int(stats.elapsed/time.Second) % 60
	return []string{
		borderTop("SHIFT DEBRIEF"),
		titleInner(fmt.Sprintf("  COMMANDS RUN  %02d       HOSTS TOUCHED  %02d", stats.commands, stats.hosts)),
		titleInner(fmt.Sprintf("  ALERTS  %02d              TIME ON SHIFT  %02d:%02d", stats.alerts, minutes, seconds)),
		titleInner("  GRADE  " + stats.grade),
		borderBottom(),
	}
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func threatLine(phase Phase) string {
	level, label := 1, "QUIET"
	switch phase {
	case PhaseSignal:
		level, label = 2, "ELEVATED"
	case PhaseCorrelation:
		level, label = 3, "HIGH"
	case PhaseContainment:
		level, label = 5, "CRITICAL"
	case PhaseReport:
		level, label = 1, "CONTAINED"
	}
	return fmt.Sprintf("THREATCON %s%s %s", strings.Repeat("#", level), strings.Repeat("-", 5-level), label)
}

func radioChatter(seed, trigger uint64, phase Phase) string {
	r := &rng{state: splitmix(seed ^ (trigger+1)*0xa0761d6478bd642f ^ uint64(phase+1)*0xe7037ed1a0b428db)}
	if r.intn(5) != 0 {
		return ""
	}
	messages := [...]string{
		"[day-shift] handoff: 3 open tickets; 1 awaiting triage",
		"[soc-lead] eyes on build-07? keep the capture rolling",
		"[netops] FRA mirror is clean; route looks synthetic",
		"[incident-4] evidence hash copied to the sealed report",
	}
	return radioPrefix + strings.TrimPrefix(messages[r.intn(len(messages))], radioPrefix)
}

type traceHop struct {
	city      string
	latencyMs int
}

var traceRoutes = [][]string{
	{"IAD", "JFK", "LHR", "FRA", "SIN"},
	{"SFO", "SEA", "NRT", "HKG", "SIN"},
	{"IAD", "LHR", "AMS", "DXB", "SIN"},
	{"JFK", "LHR", "FRA", "DXB", "HKG", "NRT"},
}

func syntheticRoute(r *rng) []traceHop {
	cityNames := traceRoutes[r.intn(len(traceRoutes))]
	hops := make([]traceHop, len(cityNames))
	for i, city := range cityNames {
		latency := r.between(2, 28)
		if i > 0 {
			latency = r.between(18, 190)
		}
		hops[i] = traceHop{city: city, latencyMs: latency}
	}
	return hops
}

func traceMapDirective(hops []traceHop) string {
	parts := make([]string, len(hops))
	for i, hop := range hops {
		parts[i] = fmt.Sprintf("%s:%d", hop.city, hop.latencyMs)
	}
	return traceMapPrefix + strings.Join(parts, ",")
}

func parseTraceMap(line string) []traceHop {
	if !strings.HasPrefix(line, traceMapPrefix) {
		return nil
	}
	items := strings.Split(strings.TrimPrefix(line, traceMapPrefix), ",")
	if len(items) < 2 || len(items) > maxTraceHops {
		return nil
	}
	hops := make([]traceHop, 0, len(items))
	for _, item := range items {
		parts := strings.Split(item, ":")
		if len(parts) != 2 || len(parts[0]) != 3 || !knownTraceCity(parts[0]) {
			return nil
		}
		if parts[1] == "" {
			return nil
		}
		for _, digit := range parts[1] {
			if digit < '0' || digit > '9' {
				return nil
			}
		}
		latency, err := strconv.Atoi(parts[1])
		if err != nil || latency < 1 || latency > maxTraceLatencyMS {
			return nil
		}
		hops = append(hops, traceHop{city: parts[0], latencyMs: latency})
	}
	return hops
}

type mapPoint struct {
	x int
	y int
}

var traceCityPoints = map[string]mapPoint{
	"SFO": {4, 3}, "SEA": {6, 1}, "IAD": {11, 2}, "JFK": {14, 3},
	"LHR": {23, 2}, "AMS": {25, 1}, "FRA": {29, 3}, "DXB": {38, 3},
	"HKG": {49, 3}, "SIN": {48, 5}, "NRT": {57, 2},
}

func knownTraceCity(city string) bool {
	_, ok := traceCityPoints[city]
	return ok
}

func traceMapRows(hops []traceHop, active int) []string {
	const (
		width  = 64
		height = 8
	)
	grid := make([][]rune, height)
	for row := range grid {
		grid[row] = []rune(strings.Repeat(" ", width))
	}
	drawWorldOutline(grid)
	for hop := 1; hop < active; hop++ {
		from, to := traceCityPoints[hops[hop-1].city], traceCityPoints[hops[hop].city]
		drawMapLeg(grid, from, to)
	}
	for index, hop := range hops[:active] {
		point := traceCityPoints[hop.city]
		grid[point.y][point.x] = rune('1' + index)
	}
	rows := []string{
		borderTop("SIMULATED ROUTE / " + fmt.Sprintf("HOP %02d OF %02d", active, len(hops))),
		"  NORTH AMERICA                  EUROPE               ASIA-PACIFIC",
	}
	for _, row := range grid {
		rows = append(rows, "  "+strings.TrimRight(string(row), " "))
	}
	rows = append(rows, traceRouteLegend(hops, active))
	if active == 1 {
		rows = append(rows, fmt.Sprintf("  LEG 01/%02d  LOCAL -> %-3s   %6.3f ms", len(hops), hops[0].city, float64(hops[0].latencyMs)))
	} else {
		from, to := hops[active-2], hops[active-1]
		rows = append(rows, fmt.Sprintf("  LEG %02d/%02d  %-3s -> %-3s   %6.3f ms", active, len(hops), from.city, to.city, float64(to.latencyMs)))
	}
	rows = append(rows, borderBottom())
	return rows
}

func traceRouteLegend(hops []traceHop, active int) string {
	parts := make([]string, 0, active)
	for index, hop := range hops[:active] {
		parts = append(parts, fmt.Sprintf("%d %s", index+1, hop.city))
	}
	return "  ROUTE  " + strings.Join(parts, "  >  ")
}

func drawWorldOutline(grid [][]rune) {
	for _, outline := range []struct {
		x, y int
		text string
	}{
		{2, 0, ".----------."},
		{1, 1, "/            \\"},
		{0, 2, "|              |"},
		{0, 3, "|              |"},
		{1, 4, "\\            /"},
		{2, 5, "`----.-----'"},
		{7, 6, "`"},
		{20, 0, ".------."},
		{19, 1, "/        \\"},
		{18, 2, "|          |"},
		{18, 3, "|          |"},
		{19, 4, "\\        /"},
		{20, 5, "`--.---'"},
		{23, 6, "`"},
		{35, 0, ".--------------------------."},
		{34, 1, "/                            \\"},
		{33, 2, "|                              |"},
		{33, 3, "|                              |"},
		{34, 4, "\\                            /"},
		{35, 5, "`-----.              .-----'"},
		{41, 6, "`------------'"},
		{46, 7, ".----."},
	} {
		putMapText(grid, outline.x, outline.y, outline.text)
	}
}

func drawMapLeg(grid [][]rune, from, to mapPoint) {
	x0, y0, x1, y1 := from.x, from.y, to.x, to.y
	dx, dy := absInt(x1-x0), -absInt(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if y0 >= 0 && y0 < len(grid) && x0 >= 0 && x0 < len(grid[y0]) && grid[y0][x0] == ' ' {
			grid[y0][x0] = '.'
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		twice := 2 * err
		if twice >= dy {
			err += dy
			x0 += sx
		}
		if twice <= dx {
			err += dx
			y0 += sy
		}
	}
}

func putMapText(grid [][]rune, x, y int, text string) {
	if y < 0 || y >= len(grid) {
		return
	}
	for index, char := range text {
		column := x + index
		if column >= 0 && column < len(grid[y]) {
			grid[y][column] = char
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func waveformText(stage int, unicode bool) string {
	if unicode {
		patterns := [...]string{
			"▁▃▂▅▂▇▃▁",
			"▂▅▃▇▂▅▃▁",
			"▁▂▅▃▇▂▅▃",
			"▂▁▂▇▂▁▂▇",
		}
		return patterns[min(stage, len(patterns)-1)]
	}
	patterns := [...]string{"._-^-. _", "..-#-.^.", ".-#..^-. ", ".-.-#.-#"}
	return patterns[min(stage, len(patterns)-1)]
}

func waveformLineText(st style) string {
	return join([]segment{{"  SIGNAL ", ansiDim}, {waveformText(3, st.unicode), ansiBrightGreen}}, st.color)
}

func dashboardLines(model Model) []string {
	seed := model.config.Seed
	r := &rng{state: splitmix(seed ^ (model.tickCount+1)*0xd1b54a32d192ed03 ^ uint64(model.phase+1))}
	cpu := r.between(18, 86)
	inbound := r.between(15, 940)
	outbound := r.between(9, 520)
	alerts := min(model.alertCount, 99)
	bar := func(value, scale int) string {
		filled := min(10, value*10/scale)
		return "[" + strings.Repeat("#", filled) + strings.Repeat("-", 10-filled) + "]"
	}
	return []string{
		borderTop("LIVE TELEMETRY"),
		titleInner(fmt.Sprintf("  CPU %02d%% %s   NET IN %03d Mb/s %s", cpu, bar(cpu, 100), inbound, bar(inbound, 1000))),
		titleInner(fmt.Sprintf("  NET OUT %03d Mb/s %s   ALERTS %02d %s", outbound, bar(outbound, 600), alerts, bar(alerts, 20))),
		borderBottom(),
	}
}
