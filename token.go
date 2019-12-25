// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/writeas/go-writeas/v2"
	"golang.org/x/crypto/ssh/terminal"
	"mellium.im/cli"
)

func cfgFile(debug *log.Logger) string {
	home, err := os.UserHomeDir()
	if err != nil {
		debug.Printf("error fetching home directory: %v", err)
	}
	if home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".writeas/user.json")
}

// loadUser returns a username and  access token by reading
// ~/.writeas/user.json, or by checking the WA_TOKEN and WA_USER environment
// variables (in that order).
func loadUser(debug *log.Logger) (username, token string) {
	tokenEnv := os.Getenv(envToken)
	userEnv := os.Getenv(envUser)

	f, err := os.Open(cfgFile(debug))
	if err != nil {
		debug.Printf("error opening %s, trying $%s instead: %v", userConfig, envToken, err)
		return userEnv, tokenEnv
	}
	d := json.NewDecoder(f)
	var user = struct {
		Token string `json:"access_token"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
	}{}
	err = d.Decode(&user)
	if err != nil {
		debug.Printf("error decoding %s, trying $%s instead: %v", userConfig, envToken, err)
		return userEnv, tokenEnv
	}

	if user.Token == "" {
		debug.Printf("no token found in %s, trying $%s instead", userConfig, envToken)
		return userEnv, tokenEnv
	}

	return user.User.Username, user.Token
}

func tokenCmd(apiBase string, torPort int, logger, debug *log.Logger) *cli.Command {
	const (
		envPass = "WA_PASS"
	)

	username, _ := loadUser(debug)
	revoke := false

	flags := flag.NewFlagSet("token", flag.ContinueOnError)
	flags.StringVar(&username, "user", username, "The username to login as, overrides $"+envUser+" and the config file")
	flags.BoolVar(&revoke, "revoke", revoke, "Revoke any listed tokens instead of generating a new one")

	return &cli.Command{
		Usage: `token [--revoke tokens...]`,
		Flags: flags,
		Description: `Generate or revoke an access token.

Reads the users password from $WA_PASS, or prompts for a password if the
environment variable is not set.`,
		Run: func(cmd *cli.Command, args ...string) error {
			if revoke {
				var err error
				for _, tok := range flags.Args() {
					c := writeas.NewClientWith(writeas.Config{
						URL:     apiBase,
						Token:   tok,
						TorPort: torPort,
					})
					debug.Printf("revoking %q…", tok)
					if e := c.LogOut(); e != nil {
						logger.Printf("error revoking %q: %v", tok, e)
						err = fmt.Errorf("some tokens could not be revoked")
					}
				}
				return err
			}

			if len(flags.Args()) > 0 {
				cmd.Help()
				return fmt.Errorf("wrong number of arguments")
			}

			pass := os.Getenv(envPass)
			switch {
			case len(pass) == 0:
				fmt.Printf("Enter password for write.as user %s: ", username)
				passBytes, err := terminal.ReadPassword(syscall.Stdin)
				if err != nil {
					return fmt.Errorf("error prompting for password, set $%s or fix TTY: %v", envPass, err)
				}
				fmt.Println()
				pass = string(passBytes)
			case len(username) == 0:
				return fmt.Errorf("A writeas-cli config file must be present or $" + envUser + " or --user must be specified to generate tokens")
			}

			c := writeas.NewClient()
			auth, err := c.LogIn(username, pass)
			if err != nil {
				return err
			}

			fmt.Println(auth.AccessToken)
			return nil
		},
	}
}
