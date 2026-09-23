package nightshift

import (
	"os"
	"testing"
	"time"
)

func TestTempoSpeedsUpWithTypingRate(t *testing.T) {
	start := time.Unix(0, 0)
	var slow, fast tempo
	slow.hit(start, 1)
	if got := slow.speed(start); got != 1 {
		t.Fatalf("one key sped up playback: %v", got)
	}
	slow.hit(start.Add(time.Second), 1)
	for i := range 12 {
		fast.hit(start.Add(time.Duration(i)*80*time.Millisecond), 1)
	}
	now := start.Add(time.Second)
	if s, f := slow.speed(now), fast.speed(now); !(1 < s && s < f && f <= maxSpeed) {
		t.Fatalf("slow %v, fast %v", s, f)
	}
	if got := fast.speed(now.Add(tempoWindow + time.Second)); got != 1 {
		t.Fatalf("speed did not settle after typing stopped: %v", got)
	}
}

func TestTempoCapsOneChunk(t *testing.T) {
	var tp tempo
	tp.hit(time.Unix(0, 0), 200)
	if len(tp.hits) != tempoChunkCap {
		t.Fatalf("hits = %d", len(tp.hits))
	}
}

func TestTypedKeysIgnoresEscapesAndControls(t *testing.T) {
	cases := map[string]int{
		"abc":           3,
		"\r\x03":        0,
		"\x1b[A":        0,
		"x\x1b[200~abc": 1,
		"\xc3\xa9":      1,
	}
	for chunk, want := range cases {
		if got := typedKeys([]byte(chunk)); got != want {
			t.Errorf("typedKeys(%q) = %d, want %d", chunk, got, want)
		}
	}
}

func TestPullQueuesBehindUnreadBytes(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})
	var seen int
	parser := &byteReader{f: reader, arrived: func(chunk []byte) { seen += typedKeys(chunk) }}
	if _, err = writer.Write([]byte("a\r")); err != nil {
		t.Fatal(err)
	}
	if kind, _ := parser.nextKey(); kind != keyTrigger {
		t.Fatalf("first key = %v", kind)
	}
	if _, err = writer.Write([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if n, err := parser.pull(); n != 1 || err != nil {
		t.Fatalf("pull = %d, %v", n, err)
	}
	writer.Close()
	var kinds []keyKind
	for {
		kind, err := parser.nextKey()
		if err != nil {
			break
		}
		kinds = append(kinds, kind)
	}
	if len(kinds) != 2 || kinds[0] != keyEnter || kinds[1] != keyTrigger {
		t.Fatalf("kinds after pull = %v", kinds)
	}
	if seen != 2 {
		t.Fatalf("arrived saw %d keys", seen)
	}
}

func TestPacerShortensWaitsWhileTyping(t *testing.T) {
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }
	var slept time.Duration
	saved := sleep
	sleep = func(d time.Duration) { slept += d }
	t.Cleanup(func() { sleep = saved })

	var waited []time.Duration
	idle := func(d time.Duration) (bool, error) { waited = append(waited, d); now = now.Add(d); return false, nil }
	tp := &tempo{}
	p := &pacer{tempo: tp, reader: &byteReader{}, poll: idle, clock: clock}
	p.wait(time.Second)
	if len(waited) != 1 || waited[0] != time.Second {
		t.Fatalf("idle wait = %v", waited)
	}
	for i := range 10 {
		tp.hit(now.Add(time.Duration(i-10)*100*time.Millisecond), 1)
	}
	waited = nil
	p.wait(time.Second)
	if len(waited) != 1 || waited[0] >= time.Second/4 {
		t.Fatalf("typing did not speed the wait: %v", waited)
	}
	unavailable := func(time.Duration) (bool, error) { return false, errInputPollingUnavailable }
	p.poll = unavailable
	p.wait(time.Second)
	if slept == 0 || slept >= time.Second/4 {
		t.Fatalf("fallback sleep = %v", slept)
	}
}
