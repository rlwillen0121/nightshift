package nightshift

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseArgsAcceptsFlagsAndClockSeed(t *testing.T) {
	fixed := time.Date(2026, 9, 22, 15, 4, 5, 42, time.UTC)
	previous := Now
	Now = func() time.Time { return fixed }
	t.Cleanup(func() { Now = previous })

	config, err := ParseArgs(nil, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if config.Seed != uint64(fixed.UnixNano()) || config.SeedProvided {
		t.Fatalf("clock seed = %#v", config)
	}

	config, err = ParseArgs([]string{"--ascii", "--no-color", "--reduced-motion", "--seed", "0"}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if config.Seed != 0 || !config.SeedProvided || !config.ASCII || !config.NoColor || !config.ReducedMotion {
		t.Fatalf("flags = %#v", config)
	}

	config, err = ParseArgs([]string{"--seed=42", "--seed=7"}, func(string) string { return "" })
	if err != nil || config.Seed != 7 {
		t.Fatalf("last seed = %#v %v", config, err)
	}
}

func TestParseArgsSanitizesInvalidArguments(t *testing.T) {
	tokens := [][]string{
		{"--ZZ-BAD-FLAG-991"},
		{"--seed"},
		{"--seed", "ZZ-BAD-SEED-991"},
		{"--seed="},
		{"--seed=ZZ-BAD-SEED-991"},
		{"positional"},
	}
	for _, args := range tokens {
		_, err := ParseArgs(args, func(string) string { return "" })
		if err == nil {
			t.Fatalf("args %q accepted", args)
		}
		if ExitCode(ExitError{Code: 2, Message: err.Error()}) != 2 {
			t.Fatal(err)
		}
		for _, arg := range args {
			if arg == "--seed" || arg == "--seed=" {
				continue
			}
			if strings.Contains(err.Error(), strings.TrimPrefix(arg, "--seed=")) && strings.Contains(arg, "ZZ-BAD") {
				t.Fatalf("error %q echoes %q", err, arg)
			}
		}
		if !strings.Contains(err.Error(), "invalid argument") {
			t.Fatal(err)
		}
	}
}

func TestParseArgsPresentationEnvironment(t *testing.T) {
	config, err := ParseArgs(nil, func(key string) string {
		if key == "NO_COLOR" {
			return "1"
		}
		return ""
	})
	if err != nil || !config.NoColor || config.ASCII || config.ReducedMotion {
		t.Fatalf("NO_COLOR = %#v %v", config, err)
	}

	config, err = ParseArgs([]string{"--seed=3"}, func(key string) string {
		if key == "TERM" {
			return "Dumb"
		}
		return ""
	})
	if err != nil || !config.NoColor || !config.ASCII || !config.ReducedMotion || config.Seed != 3 {
		t.Fatalf("dumb = %#v %v", config, err)
	}
}

func TestNonTerminalAndInvalidArgs(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})

	err = Run([]string{"--ZZ-BAD-FLAG-991"}, reader, writer, writer)
	if ExitCode(err) != 2 || strings.Contains(err.Error(), "ZZ-BAD-FLAG-991") {
		t.Fatal(err)
	}

	err = Run([]string{"--seed", "4"}, reader, writer, writer)
	if err != ErrInteractiveTerminal || ExitCode(err) != 1 {
		t.Fatal(err)
	}
}
