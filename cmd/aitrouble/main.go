// Package main is the entry point for the aitrouble CLI.
// Find where your AI integration breaks.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/osmnmlh/aitrouble/internal/doctor"
)

// version is injected at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 2
	}

	if args[0] == "--version" || args[0] == "-v" {
		fmt.Fprintf(stdout, "aitrouble %s\n", version)
		return 0
	}

	if args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		fs.SetOutput(stderr)
		envFile := fs.String("env-file", "", "Path to custom .env file")

		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		return doctor.Run(ctx, *envFile, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `aitrouble — Find where your AI integration breaks.

Usage:
  aitrouble doctor [--env-file FILE]
  aitrouble --version

Flags:
  --version   Print version and exit

Run 'aitrouble doctor --help' for command-specific flags.`)
}
