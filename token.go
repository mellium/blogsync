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

	"github.com/writeas/go-writeas/v2"
	"mellium.im/cli"
)

// getToken returns an access token by reading ~/.writeas/user.json, or by
// checking the WA_TOKEN environment variable (in that order).
func getToken(debug *log.Logger) string {
	tokenEnv := os.Getenv(envToken)

	home, err := os.UserHomeDir()
	if err != nil {
		debug.Printf("error fetching home directory: %v", err)
	}
	if home == "" {
		home = os.Getenv("HOME")
	}
	f, err := os.Open(filepath.Join(home, ".writeas/user.json"))
	if err != nil {
		debug.Printf("error opening %s, trying $%s instead: %v", userConfig, envToken, err)
		return tokenEnv
	}
	d := json.NewDecoder(f)
	var user = struct {
		Token string `json:"access_token"`
	}{}
	err = d.Decode(&user)
	if err != nil {
		debug.Printf("error decoding %s, trying $%s instead: %v", userConfig, envToken, err)
		return tokenEnv
	}

	if user.Token == "" {
		debug.Printf("no token found in %s, trying $%s instead", userConfig, envToken)
		return tokenEnv
	}

	return user.Token
}

func tokenCmd(apiBase string, torPort int, logger, debug *log.Logger) *cli.Command {
	const (
		envUser = "WA_USER"
		envPass = "WA_PASS"
	)
	var (
		username = os.Getenv(envUser)
		revoke   = false
	)
	flags := flag.NewFlagSet("token", flag.ContinueOnError)
	flags.StringVar(&username, "user", username, "The username to login as, overrides $"+envUser)
	flags.BoolVar(&revoke, "revoke", revoke, "Revoke any listed tokens instead of generating a new one")

	return &cli.Command{
		Usage: `token [--revoke tokens...]`,
		Flags: flags,
		Description: `Generate or revoke an access token.

Requires that $WA_PASS be set to the users password. This option is not provided
as a flag so that the password does not end up in the users shell history.`,
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

			pass := os.Getenv(envPass)
			switch {
			case len(pass) == 0:
				return fmt.Errorf("$" + envPass + " must be set to generate tokens")
			case len(username) == 0:
				return fmt.Errorf("$" + envUser + " or --user must be specified to generate tokens")
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
