package main

import (
	"fmt"
	"os"

	"local/nightshift/internal/nightshift"
)

func main() {
	err := nightshift.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(nightshift.ExitCode(err))
}
