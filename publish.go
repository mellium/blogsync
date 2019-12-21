// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/writeas/go-writeas/v2"
	"mellium.im/blogsync/internal/blog"
	"mellium.im/cli"
)

func publishCmd(client *writeas.Client, logger, debug *log.Logger) *cli.Command {
	var (
		collection string
		content    = "content/"
	)
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.StringVar(&collection, "collection", collection, "The default collection for posts that don't include `collection' in their frontmatter")
	flags.StringVar(&content, "content", content, "A directory containing pages and posts")

	return &cli.Command{
		Usage: "publish [options]",
		Description: fmt.Sprintf(`Publishes Markdown files to write.as.

Expects an API token to be exported as $%s.`, envToken),
		Flags: flags,
		Run: func(cmd *cli.Command, args ...string) error {
			return blog.WalkPages("content/", func(path string, info os.FileInfo, err error) error {
				debug.Printf("opening %s", path)
				fd, err := os.Open(path)
				if err != nil {
					logger.Printf("error opening %s, skipping: %v", path, err)
					return nil
				}
				defer func() {
					if err := fd.Close(); err != nil {
						debug.Printf("error closing %s: %v", path, err)
					}
				}()

				f := bufio.NewReader(fd)
				meta := &blog.Metadata{}
				err = meta.Decode(f)
				if err != nil {
					logger.Printf("error decoding metadata for %s, skipping: %v", path, err)
					return nil
				}

				draft := meta.GetBool("draft")
				if draft {
					debug.Printf("skipping draft %s", path)
					return nil
				}

				title := meta.GetString("title")
				if title == "" {
					logger.Printf("invalid or empty title in %s, skipping")
					return nil
				}
				created := meta.GetTime("date")

				if col := meta.GetString("collection"); col != "" {
					collection = col
				}

				body, err := ioutil.ReadAll(f)
				if err != nil {
					logger.Printf("error reading body from %s, skipping: %v", path, err)
					return nil
				}
				body = bytes.TrimSpace(body)
				if len(body) == 0 {
					// Apparently write.as doesn't like posts that don't have a body.
					logger.Printf("post %s has no body, skipping", path)
					return nil
				}

				_, err = client.CreatePost(&writeas.PostParams{
					Created:    created,
					Title:      title,
					Content:    string(body),
					Collection: collection,

					// TODO: Font string, get from metadata or config
					// TODO: Language *string, get from metadata or config
					// TODO: IsRTL bool, get from metadata or config
					// TODO: Updated *time.Time = get from metadata or config
					// TODO: Slug: get from metadata or configurable template
				})
				if err != nil {
					logger.Printf("error creating post from %s: %v", path, err)
					return nil
				}

				return nil
			})
		},
	}
}
