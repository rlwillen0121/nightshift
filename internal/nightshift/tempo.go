package nightshift

import "time"

// Typing tempo. The faster the operator types, the faster write-time effects
// play: every effect delay is divided by the current speed. Keys typed while
// an effect is still playing count right away, so mashing the keyboard speeds
// up the output that is already scrolling.

const (
	// tempoWindow is how far back keystrokes count toward the typing rate.
	tempoWindow = 1500 * time.Millisecond
	// tempoGain is the extra speed per key per second.
	tempoGain = 0.8
	maxSpeed  = 10.0
	// tempoChunkCap keeps one terminal read, such as a burst of key repeat,
	// from counting as a flood of separate keystrokes.
	tempoChunkCap = 3
)

type tempo struct {
	hits []time.Time
}

// hit records keys that arrived at now.
func (t *tempo) hit(now time.Time, keys int) {
	for range min(keys, tempoChunkCap) {
		t.hits = append(t.hits, now)
	}
	t.forget(now)
}

func (t *tempo) forget(now time.Time) {
	keep := 0
	for keep < len(t.hits) && now.Sub(t.hits[keep]) > tempoWindow {
		keep++
	}
	t.hits = t.hits[keep:]
}

// speed is 1 for a single deliberate key and climbs with the typing rate.
func (t *tempo) speed(now time.Time) float64 {
	t.forget(now)
	if len(t.hits) < 2 {
		return 1
	}
	rate := float64(len(t.hits)-1) / tempoWindow.Seconds()
	return min(1+rate*tempoGain, maxSpeed)
}

// typedKeys counts the keystrokes in one chunk of raw input: printable ASCII
// and UTF-8 lead bytes. Counting stops at an escape so arrow keys and pastes
// do not register as typing.
func typedKeys(chunk []byte) int {
	keys := 0
	for _, b := range chunk {
		switch {
		case b == 0x1b:
			return keys
		case b >= 0x20 && b < 0x7f, b >= 0xc0:
			keys++
		}
	}
	return keys
}

// pacer waits out effect delays at the current typing speed. While it waits
// it watches the terminal, moves arriving bytes into the key reader, and
// rescales the rest of the wait.
type pacer struct {
	tempo  *tempo
	reader *byteReader
	poll   func(time.Duration) (bool, error)
	clock  func() time.Time
}

func (p *pacer) wait(delay time.Duration) {
	for delay > 0 {
		speed := p.tempo.speed(p.clock())
		scaled := time.Duration(float64(delay) / speed)
		if scaled <= 0 {
			return
		}
		start := p.clock()
		ready, err := p.poll(scaled)
		if err != nil || !ready {
			if err != nil {
				sleep(scaled)
			}
			return
		}
		if n, pullErr := p.reader.pull(); pullErr != nil || n == 0 {
			// EOF or a hangup keeps polling ready; finish the wait plainly.
			sleep(max(scaled-p.clock().Sub(start), 0))
			return
		}
		delay -= time.Duration(float64(p.clock().Sub(start)) * speed)
	}
}
