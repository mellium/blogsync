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
	"strings"
	"text/template"

	"github.com/writeas/go-writeas/v2"
	"mellium.im/blogsync/internal/blog"
	"mellium.im/cli"
)

func publishCmd(client *writeas.Client, logger, debug *log.Logger) *cli.Command {
	var (
		collection string
		content    = "content/"
		tmpl       = "{{.Body}}"
	)
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.StringVar(&collection, "collection", collection, "The default collection for posts that don't include `collection' in their frontmatter")
	flags.StringVar(&content, "content", content, "A directory containing pages and posts")
	flags.StringVar(&tmpl, "tmpl", tmpl, "A template using Go's html/template format, to load from a file use @filename")

	return &cli.Command{
		Usage: "publish [options]",
		Description: fmt.Sprintf(`Publishes Markdown files to write.as.

Expects an API token to be exported as $%s.`, envToken),
		Flags: flags,
		Run: func(cmd *cli.Command, args ...string) error {
			compiledTmpl := template.New("root")
			var err error
			if tmplFile := strings.TrimPrefix(tmpl, "@"); tmpl != tmplFile {
				// If the template argument starts with "@" it is a filename that we
				// should load.
				compiledTmpl, err = template.ParseFiles(tmplFile)
				if err != nil {
					return fmt.Errorf("error compiling template file %s: %v", tmplFile, err)
				}
			} else {
				// Otherwise, it is a raw template and we should compile it.
				compiledTmpl, err = compiledTmpl.Parse(tmpl)
				if err != nil {
					return fmt.Errorf("error compiling template: %v", err)
				}
			}

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

				var bodyBuf strings.Builder
				err = compiledTmpl.Execute(&bodyBuf, struct {
					Body string
					Meta *blog.Metadata
				}{
					Body: string(body),
					Meta: meta,
				})
				if err != nil {
					logger.Printf("error executing template for file %s: %v", path, err)
					return nil
				}
				if bodyBuf.Len() == 0 {
					// Apparently write.as doesn't like posts that don't have a body.
					logger.Printf("post %s has no body, skipping", path)
					return nil
				}
				_, err = client.CreatePost(&writeas.PostParams{
					Created:    created,
					Title:      title,
					Content:    bodyBuf.String(),
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
