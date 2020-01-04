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
	"path"
	"strings"
	"text/template"

	"github.com/russross/blackfriday/v2"
	"github.com/writeas/go-writeas/v2"
	"mellium.im/blogsync/internal/blog"
	"mellium.im/cli"
)

const (
	defTmpl     = "{{.Body}}"
	defTmplName = "root"
)

type tmplData struct {
	Body   string
	Meta   blog.Metadata
	Config Config
}

type publishOptions struct {
	createCollections bool
	del               bool
	dryRun            bool
	force             bool
	collection        string
	content           string
	tmpl              string
}

func newPublishOpts(siteConfig Config) publishOptions {
	return publishOptions{
		collection: siteConfig.Collection,
		content:    orDef(siteConfig.Content, "content/"),
		tmpl:       orDef(siteConfig.Tmpl, defTmpl),
	}
}

func publishCmd(siteConfig Config, client *writeas.Client, logger, debug *log.Logger) *cli.Command {
	opts := newPublishOpts(siteConfig)

	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.BoolVar(&opts.del, "delete", opts.del, "Delete pages for which matching files cannot be found")
	flags.BoolVar(&opts.dryRun, "dry-run", opts.dryRun, "Perform a trial run with no changes made")
	flags.BoolVar(&opts.force, "f", opts.force, "Force publishing, even if no updates exist")
	flags.StringVar(&opts.collection, "collection", opts.collection, "The default collection for pages that don't include `collection' in their frontmatter")
	flags.StringVar(&opts.content, "content", opts.content, "A directory containing pages")
	flags.StringVar(&opts.tmpl, "tmpl", opts.tmpl, "A template using Go's html/template format, to load from a file use @filename")

	return &cli.Command{
		Usage: "publish [options]",
		Description: fmt.Sprintf(`Publishes Markdown files to write.as.

Expects an API token to be exported as $%s.`, envToken),
		Flags: flags,
		Run: func(cmd *cli.Command, args ...string) error {
			return publish(opts, siteConfig, client, logger, debug)
		},
	}
}

func publish(opts publishOptions, siteConfig Config, client *writeas.Client, logger, debug *log.Logger) error {
	var collections []writeas.Collection
	if opts.createCollections {
		colls, err := client.GetUserCollections()
		if err != nil {
			logger.Printf("error fetching existing collections: %v", err)
		}
		collections = *colls

		collections = createCollectionIfNotExist(collections, client, debug, &writeas.CollectionParams{
			Alias:       siteConfig.Collection,
			Title:       siteConfig.Title,
			Description: siteConfig.Description,
		})
	}

	compiledTmpl := template.New(defTmplName).Funcs(map[string]interface{}{
		"join": path.Join,
	})
	tmplFile := strings.TrimPrefix(opts.tmpl, "@")
	var err error
	if opts.tmpl != tmplFile {
		// If the template argument starts with "@" it is a filename that we
		// should load.
		compiledTmpl, err = compiledTmpl.ParseFiles(tmplFile)
		if err != nil {
			return fmt.Errorf("error compiling template file %s: %v", tmplFile, err)
		}
	} else {
		tmplFile = defTmplName
		// Otherwise, it is a raw template and we should compile it.
		compiledTmpl, err = compiledTmpl.Parse(opts.tmpl)
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
	// See: https://github.com/writeas/go-writeas/pull/19
	posts = *p

	err = blog.WalkPages(opts.content, func(pagePath string, info os.FileInfo, err error) error {
		debug.Printf("opening %s", pagePath)
		fd, err := os.Open(pagePath)
		if err != nil {
			logger.Printf("error opening %s, skipping: %v", pagePath, err)
			return nil
		}
		defer func() {
			if err := fd.Close(); err != nil {
				debug.Printf("error closing %s: %v", pagePath, err)
			}
		}()

		f := bufio.NewReader(fd)
		meta := make(blog.Metadata)
		header, err := meta.Decode(f)
		if err != nil {
			logger.Printf("error decoding metadata for %s, skipping: %v", pagePath, err)
			return nil
		}
		// This may seem unnecessary, but I don't plan on supporting YAML
		// headers forever to keep things simple, so go ahead and forbid
		// publishing with them to encourage people to convert their blogs over.
		if header == blog.HeaderYAML {
			logger.Printf(`file %s has a YAML header, try converting it by running "%s convert", skipping`, pagePath, os.Args[0])
			return nil
		}

		draft := meta.GetBool("draft")
		if draft {
			debug.Printf("skipping draft %s", pagePath)
			return nil
		}

		title := meta.GetString("title")
		if title == "" {
			logger.Printf("invalid or empty title in %s, skipping", pagePath)
			return nil
		}

		// Deliberately shadow collection so that we don't end up mutating the
		// options struct.
		collection := opts.collection
		if col := meta.GetString("collection"); col != "" {
			collection = col
		}

		body, err := ioutil.ReadAll(f)
		if err != nil {
			logger.Printf("error reading body from %s, skipping: %v", pagePath, err)
			return nil
		}
		body = bytes.TrimSpace(body)
		body = blackfriday.Run(body,
			blackfriday.WithNoExtensions(),
			blackfriday.WithExtensions(
				blackfriday.CommonExtensions|blackfriday.Footnotes,
			),
			blackfriday.WithRenderer(&unwrapRenderer{
				debug: debug,
				htmlRenderer: blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{
					Flags: blackfriday.FootnoteReturnLinks,
				}),
			}))

		var bodyBuf strings.Builder
		err = compiledTmpl.ExecuteTemplate(&bodyBuf, tmplFile, tmplData{
			Body:   string(body),
			Meta:   meta,
			Config: siteConfig,
		})
		if err != nil {
			logger.Printf("error executing template for file %s: %v", pagePath, err)
			return nil
		}
		if bodyBuf.Len() == 0 {
			// Apparently write.as doesn't like posts that don't have a body.
			logger.Printf("post %s has no body, skipping", pagePath)
			return nil
		}

		slug := blog.Slug(pagePath, meta)
		var existingPost *writeas.Post
		for i, post := range posts {
			var postCollection string
			if post.Collection != nil {
				postCollection = post.Collection.Alias
			}

			if slug == post.Slug && collection == postCollection {
				existingPost = &post
				posts = append(posts[:i], posts[i+1:]...)
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
		if lang == "" {
			lang = siteConfig.Language
		}
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

		var skipUpdate bool
		if existingPost == nil {
			debug.Printf("publishing %s from %s", slug, pagePath)
		} else {
			if eqParams(existingPost, params) && !opts.force {
				debug.Printf("no updates needed for %s, skipping", slug)
				skipUpdate = true
			} else {
				debug.Printf("updating /%s (%q) from %s", slug, postID, pagePath)
			}
		}

		if !opts.dryRun && !skipUpdate {
			if opts.createCollections {
				collections = createCollectionIfNotExist(collections, client, debug, &writeas.CollectionParams{
					Alias: params.Collection,
					Title: params.Collection,
				})
			}
			if postID == "" {
				post, err := client.CreatePost(params)
				if err != nil {
					logger.Printf("error creating post from %s: %v", pagePath, err)
					return nil
				}
				postID = post.ID
			} else {
				// Write.as returns a generic 500 error if you set Created when
				// updating a post, even if it's unchanged.
				params.Created = nil
				post, err := client.UpdatePost(postID, postTok, params)
				if err != nil {
					logger.Printf("error updating post %q from %s: %v", postID, pagePath, err)
					return nil
				}
				postID = post.ID
			}
		}

		// Right now there is no way to check if a post is pinned, so we have to
		// assume that all posts may be pinned and always attempt to unpin them
		// then re-pin any that should actually be pinned every time.
		// This is not ideal.
		debug.Printf("attempting to unpin post %s…", slug)
		if !opts.dryRun {
			err = client.UnpinPost(collection, &writeas.PinnedPostParams{
				ID: postID,
			})
			if err != nil {
				debug.Printf("error unpinning post %s: %v", slug, err)
			}
		}

		pin, pinExists := meta["pin"]
		ipin, pinInt := pin.(int64)
		if pinExists && pinInt {
			debug.Printf("attempting to pin post %s to position %d…", slug, int(ipin))
			if !opts.dryRun {
				err = client.PinPost(collection, &writeas.PinnedPostParams{
					ID:       postID,
					Position: int(ipin),
				})
				if err != nil {
					debug.Printf("error pinning post %s to position %d: %v", slug, int(ipin), err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Delete remaining posts for which we couldn't find a matching file.
	for _, post := range posts {
		if opts.del {
			debug.Printf("no file found matching post %q, deleting", post.Slug)
			if !opts.dryRun {
				err := client.DeletePost(post.ID, post.Token)
				if err != nil {
					logger.Printf("error deleting post %q: %v", post.Slug, err)
				}
			}
			continue
		}
		logger.Printf("no file found matching post %q, re-run with --delete to remove", post.Slug)
	}

	return nil
}

func createCollectionIfNotExist(colls []writeas.Collection, client *writeas.Client, debug *log.Logger, coll *writeas.CollectionParams) []writeas.Collection {
	for _, c := range colls {
		if c.Alias == coll.Alias {
			return colls
		}
	}
	debug.Printf("creating collection %s…", coll.Alias)
	newColl, err := client.CreateCollection(coll)
	if err != nil {
		debug.Printf("error creating collection %s: %v", coll.Alias, err)
	}
	if coll != nil {
		colls = append(colls, *newColl)
	}
	return colls
}
