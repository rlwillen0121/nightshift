package nightshift

import (
	"strings"
	"time"
)

const (
	phaseCount = 5
	tickEvery  = 180 * time.Millisecond
)

type Phase int

const (
	PhaseBoot Phase = iota
	PhaseSignal
	PhaseCorrelation
	PhaseContainment
	PhaseReport
)

// Model is the simulation state. Operator key bytes never reach it.
type Model struct {
	config       Config
	phase        Phase
	ready        bool
	finale       bool
	started      time.Time
	elapsed      time.Duration
	lastHint     string
	triggerCount uint64
	tickCount    uint64
	commandsRun  int
	alertCount   int
	hostsTouched []string
	cosmetic     cosmetic
	transcript   []string
}

type cosmetic struct {
	Callsign string
}

func NewModel(config Config) Model {
	if !config.SeedProvided && config.Seed == 0 {
		config.Seed = uint64(Now().UnixNano())
	}
	model := Model{
		config:   config,
		started:  Now(),
		cosmetic: cosmeticFor(config.Seed),
	}
	model.transcript = append(headerLines(model.config.Seed, model.cosmetic.Callsign), bootLines()...)
	model.transcript = append(model.transcript, phaseBanner(PhaseBoot)...)
	return model
}

func splitmix(seed uint64) uint64 {
	value := seed + 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func cosmeticFor(seed uint64) cosmetic {
	value := splitmix(seed)
	callSigns := [...]string{"KESTREL", "MOTHLIGHT", "APOGEE", "STARLING"}
	return cosmetic{Callsign: callSigns[value%uint64(len(callSigns))]}
}

func (m Model) Transcript() string {
	return strings.Join(m.transcript, "\n")
}

// Tick records elapsed time only. It does not print, and it does not change
// the phase or the transcript.
func (m Model) Tick(now time.Time) Model {
	m.tickCount++
	m.elapsed = m.elapsedAt(now)
	return m
}

func (m Model) elapsedAt(now time.Time) time.Duration {
	elapsed := now.Sub(m.started)
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (m Model) Handle(kind keyKind) (Model, []string, bool) {
	switch kind {
	case keyQuit:
		return m, nil, true
	case keyEnter:
		next, lines := m.Enter()
		return next, lines, false
	case keyTrigger:
		next, lines := m.Trigger()
		return next, lines, false
	default:
		return m, nil, false
	}
}

func (m Model) Enter() (Model, []string) {
	if m.finale {
		m.phase = PhaseBoot
		m.ready = false
		m.finale = false
		m.lastHint = ""
		m.triggerCount = 0
		m.tickCount = 0
		m.commandsRun = 0
		m.alertCount = 0
		m.hostsTouched = nil
		m.started = Now()
		m.elapsed = 0
		return m.appendLines(restartLines(m.config.Seed))
	}
	if !m.ready {
		m.lastHint = phaseCopies[m.phase].Hint
		return m.appendLines([]string{"// " + m.lastHint})
	}
	if m.phase == PhaseReport {
		m.elapsed = m.elapsedAt(Now())
		m.finale = true
		m.lastHint = "MISSION COMPLETE // ENTER RESTARTS THE SEEDED SIMULATION"
		return m.appendLines(finaleLines(m.missionStats()))
	}
	from := m.phase
	m.phase++
	m.ready = false
	m.lastHint = ""
	return m.appendLines(advanceLines(from, m.phase))
}

func (m Model) Trigger() (Model, []string) {
	if m.finale {
		return m, nil
	}
	// A blank line sets each burst apart as its own block.
	burst := burstLines(m.config.Seed, m.triggerCount, m.phase)
	for _, line := range burst {
		if strings.HasPrefix(line, promptPrefix) {
			m.commandsRun++
		}
		if strings.HasPrefix(line, warnPrefix) {
			m.alertCount++
		}
	}
	host := burstTarget(m.config.Seed, m.triggerCount, m.phase).tag
	if !containsString(m.hostsTouched, host) {
		m.hostsTouched = append(m.hostsTouched, host)
	}
	if chatter := radioChatter(m.config.Seed, m.triggerCount, m.phase); chatter != "" {
		burst = append(burst, chatter)
	}
	lines := append([]string{""}, burst...)
	m.triggerCount++
	m.ready = true
	m.lastHint = ""
	return m.appendLines(lines)
}

func (m Model) appendLines(lines []string) (Model, []string) {
	m.transcript = append(m.transcript, lines...)
	return m, lines
}
