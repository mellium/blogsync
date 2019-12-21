// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

// The blog command exports posts in Markdown format to write.as.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"mellium.im/cli"
)

func main() {
	// Setup logging
	logger := log.New(os.Stderr, "", log.LstdFlags)
	debug := log.New(ioutil.Discard, "DEBUG ", log.LstdFlags)

	// Setup flags (but defer parsing them until after the help system is setup so
	// that we can set the usage function to use the help system).
	var (
		verbose bool
	)
	flags := flag.NewFlagSet("blog", flag.ContinueOnError)
	flags.BoolVar(&verbose, "v", false, "Enables verbose debug logging")

	// Setup the CLI
	cmds := &cli.Command{
		Usage: fmt.Sprintf("%s <command>", os.Args[0]),
		Flags: flags,
		Commands: []*cli.Command{{
			Usage:       `export [arguments]`,
			Description: `Exports Markdown files to posts on write.as.`,
			Run: func(c *cli.Command, args ...string) error {
				panic("export: not yet implemented")
				return nil
			},
		}},
	}

	// Setup the help system and add it to the CLI and flag handling error logic.
	helpCmd := cli.Help(cmds)
	cmds.Commands = append(cmds.Commands, helpCmd)
	flags.Usage = func() {
		err := helpCmd.Run(helpCmd)
		if err != nil {
			logger.Fatal(err)
		}
	}

	// Parse flags and perform setup based on global flags such as enabling
	// verbose logging.
	err := flags.Parse(os.Args[1:])
	switch err {
	case flag.ErrHelp:
		// We've already printed the usage line, but it was expected so exit with
		// success.
		return
	case nil:
	default:
		logger.Fatalf("error while parsing flags: %v", err)
	}

	if verbose {
		debug.SetOutput(os.Stderr)
	}

	// Execute any commands that are left over on the command line after flags
	// have been handled.
	// This may perform further flag parsing.
	err = cmds.Exec(flags.Args()...)
	switch err {
	case cli.ErrNoRun:
		// If no command was passed, just show help output.
		err := helpCmd.Run(helpCmd)
		if err != nil {
			logger.Fatal(err)
		}
		os.Exit(2)
	case cli.ErrInvalidCmd:
		err := helpCmd.Run(helpCmd)
		if err != nil {
			logger.Fatal(err)
		}
		os.Exit(3)
	case nil:
		// Nothing to do here, we're done!
	default:
		logger.Printf("error executing command: %v", err)
		os.Exit(4)
	}
}
