// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

// The blogsync command exports posts in Markdown format to write.as.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/writeas/go-writeas/v2"
	"mellium.im/cli"
)

const (
	envAPIBase = "WA_URL"
	envToken   = "WA_TOKEN"
	envTorPort = "TOR_SOCKS_PORT"
)

func main() {
	// Setup logging
	logger := log.New(os.Stderr, "", log.LstdFlags)
	debug := log.New(ioutil.Discard, "DEBUG ", log.LstdFlags)

	// Setup flags
	var (
		verbose = false
		torPort = intEnv(envTorPort)
		apiBase = envOrDef(envAPIBase, "https://write.as/api")
	)
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.Usage = func() {}
	flags.BoolVar(&verbose, "v", false, "Enables verbose debug logging")
	flags.IntVar(&torPort, "orport", torPort, "The port of a local Tor SOCKS proxy, overrides $"+envTorPort)
	flags.StringVar(&apiBase, "url", apiBase, "The base API URL, overrides $"+envAPIBase)

	// Parse flags and perform setup based on global flags such as enabling
	// verbose logging and creating a write.as client.
	var showHelp bool
	err := flags.Parse(os.Args[1:])
	switch err {
	case flag.ErrHelp:
		showHelp = true
	case nil:
	default:
		showHelp = true
		logger.Printf("error while parsing flags: %v", err)
	}

	if verbose {
		debug.SetOutput(os.Stderr)
	}

	client := writeas.NewClientWith(writeas.Config{
		URL:     apiBase,
		Token:   os.Getenv(envToken),
		TorPort: torPort,
	})

	// Setup the CLI
	cmds := &cli.Command{
		Usage: fmt.Sprintf(`%s <command>

Most commands expect a Write.as API token to be exported as $%s.
To get a token, use the "token" command.`, os.Args[0], envToken),
		Flags: flags,
		Commands: []*cli.Command{
			publishCmd(client, logger, debug),
			tokenCmd(apiBase, torPort, logger, debug),
		},
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

	if showHelp {
		flags.Usage()
		os.Exit(1)
	}

	// Execute any commands that are left over on the command line after flags
	// have been handled.
	// This may perform further flag parsing.
	debug.Printf("running subcommand: %+v", flags.Args())
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
