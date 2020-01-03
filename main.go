// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

// The blogsync command exports posts in Markdown format to write.as.
//
// For more information, try running:
//
//     blogsync help
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/writeas/go-writeas/v2"
	"mellium.im/cli"
)

const (
	envAPIBase = "WA_URL"
	envToken   = "WA_TOKEN"
	envUser    = "WA_USER"
	envTorPort = "TOR_SOCKS_PORT"

	userConfig = "~/.writeas/user.json"
)

// Config holds site configuration.
type Config struct {
	BaseURL     string `toml:"BaseURL"`
	Collection  string `toml:"Collection"`
	Content     string `toml:"Content"`
	Description string `toml:"Description"`
	Language    string `toml:"Language"`
	Title       string `toml:"Title"`
	Tmpl        string `toml:"Tmpl"`

	Author []struct {
		Name  string `toml:"Name"`
		Email string `toml:"Email"`
		URI   string `toml:"URI"`
	} `toml:"Author"`

	Params map[string]interface{} `toml:"Params"`
}

func main() {
	// Setup logging
	logger := log.New(os.Stderr, "", log.LstdFlags)
	debug := log.New(ioutil.Discard, "DEBUG ", log.LstdFlags)

	// Setup flags
	var (
		verbose = false
		torPort = intEnv(envTorPort)
		apiBase = envOrDef(envAPIBase, "https://write.as/api")
		config  = ""
	)
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.Usage = func() {}
	flags.BoolVar(&verbose, "v", false, "Enables verbose debug logging")
	flags.IntVar(&torPort, "orport", torPort, "The port of a local Tor SOCKS proxy, overrides $"+envTorPort)
	flags.StringVar(&apiBase, "url", apiBase, "The base API URL, overrides $"+envAPIBase)
	flags.StringVar(&config, "config", config, `The config file to load (defaults to "config.toml"`)

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

	siteConfig := Config{}
	cfgFile := config
	if cfgFile == "" {
		cfgFile = "config.toml"
	}
	_, err = toml.DecodeFile(cfgFile, &siteConfig)
	if err != nil && config != "" {
		logger.Fatalf("error loading %s: %v", cfgFile, err)
	}

	_, tok := loadUser(debug)
	client := writeas.NewClientWith(writeas.Config{
		URL:     apiBase,
		Token:   tok,
		TorPort: torPort,
	})

	// Setup the CLI
	cmds := &cli.Command{
		Usage: fmt.Sprintf(`%s <command>

Most commands expect to find a Write.as API token in the writeas-cli config file
(normally %s) or exported as $%s.
To get a token, use the "token" command.`, os.Args[0], userConfig, envToken),
		Flags: flags,
		Commands: []*cli.Command{
			// Sub-commands
			collectionsCmd(client, logger, debug),
			convertCmd(logger, debug),
			previewCmd(siteConfig, logger, debug),
			publishCmd(false, siteConfig, client, logger, debug),
			tokenCmd(apiBase, torPort, logger, debug),

			// Help articles
			tmplArticle(),
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
