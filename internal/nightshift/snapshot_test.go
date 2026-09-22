package nightshift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeaderAndFirstBurstSnapshot(t *testing.T) {
	config := Config{Seed: 42, SeedProvided: true}
	model := NewModel(config)
	opening := len(model.transcript)
	var burst []string
	model, burst = model.Trigger()
	if !validBurst(burst) {
		t.Fatalf("burst length %d", len(burst))
	}
	if len(model.transcript) != opening+len(burst) {
		t.Fatal("snapshot is not header plus one burst")
	}
	got := model.Transcript() + "\n"
	if !strings.Contains(got, "SEED 000000000000002A") || !strings.Contains(got, "CALLSIGN "+cosmeticFor(42).Callsign) || !strings.Contains(got, wordmarkRows()[0]) {
		t.Fatal(got)
	}
	path := filepath.Join("testdata", "header-burst.golden")
	if os.Getenv("NIGHTSHIFT_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
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
	for _, cfg := range []Config{
		{Seed: 42, SeedProvided: true, ASCII: true},
		{Seed: 42, SeedProvided: true, NoColor: true},
		{Seed: 42, SeedProvided: true, ReducedMotion: true},
	} {
		other := NewModel(cfg)
		other, _ = other.Trigger()
		if other.Transcript()+"\n" != got {
			t.Fatalf("flags changed snapshot %#v", cfg)
		}
	}
}
