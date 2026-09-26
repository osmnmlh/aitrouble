// Package main is the entry point for the aitrouble CLI.
// Find where your AI integration breaks.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is injected at build time via -ldflags.
var version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("aitrouble %s\n", version)
		os.Exit(0)
	}

	// Subcommand dispatch — full implementation coming in M1.
	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "doctor":
		fmt.Println("aitrouble doctor — not yet implemented. Coming in M3.")
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`aitrouble — Find where your AI integration breaks.

Usage:
  aitrouble doctor   [--env-file FILE] [--only LAYER] [--format FORMAT]
  aitrouble --version

Flags:
  --version   Print version and exit

Run 'aitrouble <command> --help' for command-specific flags.`)
}
