// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"mellium.im/blogsync/internal/blog"
	"mellium.im/cli"
)

func convertCmd(logger, debug *log.Logger) *cli.Command {
	var (
		dryRun  = false
		content = "content/"
	)
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.BoolVar(&dryRun, "dry-run", dryRun, "Perform a trial run with no changes made")
	flags.StringVar(&content, "content", content, "A directory containing pages and posts")

	return &cli.Command{
		Usage: "convert [options]",
		Flags: flags,
		Description: `Converts post metadata to the required format.

Convert searches the content directory for posts and performs the following
transformations on any posts it finds:

	- Convert YAML frontmatter beginning with --- to TOML
	- Convert "date" and "lastmod" fields to TOML date types
	- Ensure the body has a single leading and trailing \n and trim other leading
	  and trailing whitespace

This command does not attempt to preserve files if it fails. Be sure to make a
backup or commit all files to source control before converting them.`,
		Run: func(cmd *cli.Command, args ...string) error {
			return blog.WalkPages(content, func(path string, info os.FileInfo, err error) error {
				fd, err := os.OpenFile(path, os.O_RDWR, 0666)
				if err != nil {
					logger.Printf("error opening %s, skipping: %v", path, err)
					return nil
				}
				defer func() {
					if err := fd.Close(); err != nil {
						debug.Printf("error closing %s: %v", path, err)
					}
				}()

				var madeChanges bool
				f := bufio.NewReader(fd)
				meta := make(blog.Metadata)
				header, err := meta.Decode(f)
				if err != nil {
					logger.Printf("error decoding metadata for %s, skipping: %v", path, err)
					return nil
				}

				if header != "+++\n" {
					madeChanges = true
					debug.Printf("converting non-TOML frontmatter in %s…", path)
				}
				const (
					dateKey   = "date"
					updateKey = "lastmod"
				)
				if date, ok := meta[dateKey]; ok {
					if _, ok := date.(string); ok {
						debug.Printf("converting string date in %s…", path)
						meta[dateKey] = meta.GetTime(dateKey)
						madeChanges = true
					}
				}
				if date, ok := meta[updateKey]; ok {
					if _, ok := date.(string); ok {
						debug.Printf("converting string lastmod in %s…", path)
						meta[updateKey] = meta.GetTime(updateKey)
						madeChanges = true
					}
				}

				body, err := ioutil.ReadAll(f)
				if err != nil {
					logger.Printf("error reading body from %s, skipping: %v", path, err)
					return nil
				}
				prevBody := string(body)
				body = bytes.TrimSpace(body)
				if len(body) > 0 {
					body = append([]byte{'\n'}, body...)
					body = append(body, '\n')
				}
				if !bytes.Equal([]byte(prevBody), body) {
					logger.Printf("trimming body on %s…", path)
					madeChanges = true
				}

				// If there are no changes to make, don't bother rewriting the file
				// which could cause a diff because we can't guarantee the order of
				// metadata in the frontmatter.
				if !madeChanges || dryRun {
					return nil
				}

				_, err = fd.Seek(0, io.SeekStart)
				if err != nil {
					return fmt.Errorf("error seeking in %s: %v", path, err)
				}
				err = fd.Truncate(0)
				if err != nil {
					return fmt.Errorf("error truncating %s: %v", path, err)
				}

				// Write the new metadata to the file
				_, err = fmt.Fprint(fd, "+++\n")
				if err != nil {
					logger.Printf("could not write header start to %s: %v", path, err)
				}
				e := toml.NewEncoder(fd)
				err = e.Encode(meta)
				if err != nil {
					logger.Printf("error encoding TOML in %s: %v", path, err)
				}
				_, err = fmt.Fprint(fd, "+++\n")
				if err != nil {
					logger.Printf("could not write header close to %s: %v", path, err)
				}

				// If there is no body, we're done. Don't bother adding an extra
				// trailing newline.
				if len(body) == 0 {
					return nil
				}

				_, err = fd.Write(body)
				if err != nil {
					logger.Printf("failed to write body to %s: %v", path, err)
				}
				return nil
			})
		},
	}
}
