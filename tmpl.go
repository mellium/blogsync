// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"mellium.im/blogsync/internal/blog"
	"mellium.im/cli"
)

const helpIndent = "\t"

func tmplArticle() *cli.Command {
	defTmplData := tmplData{
		Meta: make(blog.Metadata),
		Config: Config{
			Params: make(map[string]interface{}),
		},
	}
	var encodedTmplData strings.Builder
	e := toml.NewEncoder(&encodedTmplData)
	e.Indent = ""
	err := e.Encode(defTmplData)
	if err != nil {
		panic(fmt.Errorf(`Error marshaling TOML for help output.  This should never
happen.  If you see this message, please report it and include the following
error message in your report:

%w`, err))
	}

	// This is all very inefficient, but it's a tiny string in a help article so
	// I'm not sure that we care, let's go for simple debugging and readability.
	lines := strings.Split(encodedTmplData.String(), "\n")
	for i, line := range lines {
		lines[i] = "\t" + line
	}
	indentedTmplData := strings.Join(lines, "\n")

	return &cli.Command{
		Usage: "templates",
		Description: fmt.Sprintf(`Writing templates that can be applied to pages.

The publish command supports specifying a template that can be applied to a post
before it is published.  These templates use Go's text/template package, a
description of which can be found here:

%shttps://golang.org/pkg/text/template/

Templates are passed the following data (represented in TOML format):

%s

The body field contains the markdown from the page being published, that is,
everything after the frontmatter.  The Meta table contains the fields from the
TOML frontmatter.  The Config field contains values loaded from the site config
file.  If you want to add arbitrary values to the config file they must be in
the Params section.

If no template is specified when publishing, the body is published as-is using
the template:

%[1]s%[3]s`, helpIndent, indentedTmplData, defTmpl),
	}
}
