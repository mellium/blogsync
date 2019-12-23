// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"

	"github.com/writeas/go-writeas/v2"
	"mellium.im/cli"
)

func collectionsCmd(client *writeas.Client, logger, debug *log.Logger) *cli.Command {
	return &cli.Command{
		Usage:       "collections",
		Description: `List collections owned by the authenticated user.`,
		Run: func(cmd *cli.Command, args ...string) error {
			colls, err := client.GetUserCollections()
			if err != nil {
				return err
			}

			for _, coll := range *colls {
				fmt.Printf("%+v\n", coll)
			}
			return nil
		},
	}
}
