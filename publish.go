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
	"time"

	"github.com/writeas/go-writeas/v2"
	"mellium.im/blogsync/internal/blog"
	"mellium.im/cli"
)

func publishCmd(siteConfig Config, client *writeas.Client, logger, debug *log.Logger) *cli.Command {
	var (
		collection = ""
		dryRun     = false
		content    = "content/"
		tmpl       = "{{.Body}}"
	)
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.BoolVar(&dryRun, "dry-run", dryRun, "Perform a trial run with no changes made")
	flags.StringVar(&collection, "collection", siteConfig.Collection, "The default collection for posts that don't include `collection' in their frontmatter")
	flags.StringVar(&content, "content", content, "A directory containing pages and posts")
	flags.StringVar(&tmpl, "tmpl", orDef(siteConfig.Tmpl, tmpl), "A template using Go's html/template format, to load from a file use @filename")

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

			var posts []writeas.Post
			p, err := client.GetUserPosts()
			if err != nil {
				return fmt.Errorf("error fetching users posts: %v", err)
			}
			// For now, the writeas SDK returns things with a lot of unnecessary
			// indirection that makes the library hard to use.
			// Go ahead and unwrap this and we can remove this workaround if they ever
			// fix it.
			posts = *p

			return blog.WalkPages(content, func(path string, info os.FileInfo, err error) error {
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
				meta := make(blog.Metadata)
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
					logger.Printf("invalid or empty title in %s, skipping", path)
					return nil
				}

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
					Meta blog.Metadata
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

				slug := blog.Slug(path, meta)
				var existingPost *writeas.Post
				for _, post := range posts {
					var postCollection string
					if post.Collection != nil {
						postCollection = post.Collection.Alias
					}

					if slug == post.Slug && collection == postCollection {
						existingPost = &post
						break
					}
				}

				created := timeOrDef(meta.GetTime("publishDate"), meta.GetTime("date"))
				createdPtr := &created
				if created.IsZero() {
					createdPtr = nil
				}
				rtl := meta.GetBool("rtl")
				lang := meta.GetString("lang")
				updated := timeOrDef(meta.GetTime("lastmod"), created)

				var postID, postTok string
				if existingPost != nil {
					postID = existingPost.ID
					postTok = existingPost.Token
				}
				params := &writeas.PostParams{
					ID:    postID,
					Token: postTok,

					Content:  bodyBuf.String(),
					Created:  createdPtr,
					Font:     orDef(meta.GetString("font"), "norm"),
					IsRTL:    &rtl,
					Language: &lang,
					Slug:     slug,
					Title:    title,
					Updated:  &updated,

					Collection: collection,
				}

				if existingPost == nil {
					debug.Printf("publishing %s from %s", slug, path)
				} else {
					var created, updated time.Time
					if params.Updated != nil {
						updated = *params.Updated
					}
					if params.Created != nil {
						created = *params.Created
					}
					cmpPost := writeas.Post{
						ID:       params.ID,
						Slug:     params.Slug,
						Token:    params.Token,
						Font:     params.Font,
						Language: params.Language,
						RTL:      params.IsRTL,
						Created:  created,
						Updated:  updated,
						Title:    params.Title,
						Content:  params.Content,
						Views:    existingPost.Views,
						// TODO: what is this?
						Listed: existingPost.Listed,
						// TODO: implement tags
						Tags:      existingPost.Tags,
						Images:    existingPost.Images,
						OwnerName: existingPost.OwnerName,
						// TODO: implement collection changing if ID is set on the post
						Collection: existingPost.Collection,
					}
					if eqPost(&cmpPost, existingPost) {
						debug.Printf("no updates needed for %s, skipping", slug)
					} else {
						debug.Printf("updating /%s (%q) from %s", slug, postID, path)
					}
				}

				if dryRun {
					return nil
				}

				if postID == "" {
					_, err = client.CreatePost(params)
					if err != nil {
						logger.Printf("error creating post from %s: %v", path, err)
						return nil
					}
					return nil
				}

				// Write.as returns a generic 500 error if you set Created, even if it's
				// unchanged.
				params.Created = nil
				_, err = client.UpdatePost(postID, postTok, params)
				if err != nil {
					logger.Printf("error updating post %q from %s: %v", postID, path, err)
					return nil
				}
				return nil
			})
		},
	}
}
