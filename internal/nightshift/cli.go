package nightshift

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the complete presentation configuration. It contains no operator input.
type Config struct {
	Seed          uint64
	SeedProvided  bool
	ASCII         bool
	NoColor       bool
	ReducedMotion bool
}

// ExitError carries the process status for a sanitized CLI failure.
type ExitError struct {
	Code    int
	Message string
}

func (e ExitError) Error() string { return e.Message }

// ExitCode maps a run error to a process status. Invalid arguments are 2.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	return 1
}

func ParseArgs(args []string, env func(string) string) (Config, error) {
	config := Config{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--ascii":
			config.ASCII = true
		case arg == "--no-color":
			config.NoColor = true
		case arg == "--reduced-motion":
			config.ReducedMotion = true
		case arg == "--seed":
			if index+1 >= len(args) {
				return Config{}, fmt.Errorf("invalid argument: --seed requires an unsigned integer")
			}
			index++
			seed, err := strconv.ParseUint(args[index], 10, 64)
			if err != nil {
				return Config{}, fmt.Errorf("invalid argument: --seed requires an unsigned integer")
			}
			config.Seed = seed
			config.SeedProvided = true
		case strings.HasPrefix(arg, "--seed="):
			seed, err := strconv.ParseUint(strings.TrimPrefix(arg, "--seed="), 10, 64)
			if err != nil {
				return Config{}, fmt.Errorf("invalid argument: --seed requires an unsigned integer")
			}
			config.Seed = seed
			config.SeedProvided = true
		default:
			return Config{}, fmt.Errorf("invalid argument: unsupported flag")
		}
	}
	if env == nil {
		env = os.Getenv
	}
	if env("NO_COLOR") != "" || strings.EqualFold(env("TERM"), "dumb") {
		config.NoColor = true
	}
	if strings.EqualFold(env("TERM"), "dumb") {
		config.ASCII = true
		config.ReducedMotion = true
	}
	if !config.SeedProvided {
		config.Seed = uint64(Now().UnixNano())
	}
	return config, nil
}
