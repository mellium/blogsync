// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/russross/blackfriday/v2"
)

// unwrapRenderer is a blackfriday.Renderer that generates a markdown document
// that is semantically the same as the input document except with hard wrapping
// removed from text nodes.
// We have to do this because of the jank way in which Write.as bungles even the
// core markdown standard instead of just handling newlines differently in the
// web WYSIWYG editor.
type unwrapRenderer struct {
	listLevel  int
	quoteLevel int

	debug *log.Logger
}

func (*unwrapRenderer) RenderFooter(w io.Writer, ast *blackfriday.Node) {}
func (*unwrapRenderer) RenderHeader(w io.Writer, ast *blackfriday.Node) {}
func (rend *unwrapRenderer) RenderNode(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	switch node.Type {
	case blackfriday.Document:
		return blackfriday.GoToNext
	case blackfriday.List:
		return rend.renderList(w, node, entering)
	case blackfriday.Item:
		return rend.renderItem(w, node, entering)
	case blackfriday.Paragraph:
		return rend.renderPar(w, node, entering)
	case blackfriday.Heading:
		return rend.renderHeading(w, node, entering)
	case blackfriday.HorizontalRule:
		return rend.renderHR(w, node, entering)
	case blackfriday.Emph:
		return rend.renderSpan("*", w, node, entering)
	case blackfriday.Strong:
		return rend.renderSpan("**", w, node, entering)
	case blackfriday.Del:
		return rend.renderSpan("~~", w, node, entering)
	case blackfriday.Link:
		return rend.renderLink(w, node, entering)
	case blackfriday.Image:
		return rend.renderImage(w, node, entering)
	case blackfriday.BlockQuote:
		return rend.renderBlockQuote(w, node, entering)
	case blackfriday.Text:
		return rend.renderText(w, node, entering)
	case blackfriday.HTMLBlock:
		return rend.renderHTMLBlock(w, node, entering)
	case blackfriday.CodeBlock:
		return rend.renderCodeBlock(w, node, entering)
	case blackfriday.Code:
		return rend.renderCode(w, node, entering)
	case blackfriday.Hardbreak:
		return rend.renderHardBreak(w, node, entering)
	case blackfriday.HTMLSpan:
		return rend.renderHTMLSpan(w, node, entering)
	}

	// Softbreaks are not supported by blackfriday and this message should never
	// be hit for them, see: https://github.com/russross/blackfriday/issues/315
	// TODO: support tables
	rend.debug.Printf("unsupported markdown node %s found", node.Type)
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderLink(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if entering {
		fmt.Fprintf(w, "[")
		return blackfriday.GoToNext
	}

	fmt.Fprintf(w, "](%s %q)", node.LinkData.Destination, node.LinkData.Title)
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderImage(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if entering {
		fmt.Fprintf(w, "![")
		return blackfriday.GoToNext
	}

	fmt.Fprintf(w, "](%s %q)", node.LinkData.Destination, node.LinkData.Title)
	return blackfriday.GoToNext
}

func (rend *unwrapRenderer) renderList(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if entering {
		rend.listLevel++
		return blackfriday.GoToNext
	}

	rend.listLevel--
	return blackfriday.GoToNext
}

func (rend *unwrapRenderer) renderItem(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return blackfriday.GoToNext
	}
	for i := 0; i < rend.listLevel; i++ {
		fmt.Fprintf(w, "  ")
	}
	fmt.Fprintf(w, "* ")
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderHeading(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		io.WriteString(w, "\n")
		return blackfriday.GoToNext
	}

	// Convert all headings to # style.
	// The specific style doesn't matter as far as write.as is concerned and
	// since blackfriday doesn't give us the raw header literal we just make up
	// that they're all # style to avoid having to do more work.
	for i := 0; i < node.HeadingData.Level; i++ {
		io.WriteString(w, "#")
	}
	io.WriteString(w, " ")
	return blackfriday.GoToNext
}

func (rend *unwrapRenderer) renderBlockQuote(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		io.WriteString(w, "\n")
		rend.quoteLevel--
		return blackfriday.GoToNext
	}

	// For the actual rendering of the ">" marker, see renderPar, this just tracks
	// the state of block quotes starting and ending. If nothing is put inside of
	// them, they don't render at all.
	rend.quoteLevel++
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderText(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return blackfriday.GoToNext
	}

	_, err := w.Write(bytes.ReplaceAll(node.Literal, []byte{'\n'}, []byte{' '}))
	if err != nil {
		panic(fmt.Errorf("error writing markdown to buffer: %w"))
	}
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderHardBreak(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return blackfriday.GoToNext
	}

	// Convert double spaces at the end of lines into an actual line break which
	// is what write.as expects.
	io.WriteString(w, "\n")
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderHTMLBlock(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return blackfriday.GoToNext
	}

	if len(node.Literal) > 0 {
		_, err := w.Write(node.Literal)
		if err != nil {
			panic(fmt.Errorf("error writing markdown to buffer: %w"))
		}
	}
	// If a paragraph or other markdown element comes after HTML we need two line
	// breaks to make sure it's part of a new block and gets wrapped in a <p> or
	// similar. This is unlike most markdown elements which are in a paragraph and
	// thus the renderPar method will add a line break afterwards and where only a
	// single line break will suffice.
	io.WriteString(w, "\n\n")

	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderHTMLSpan(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return blackfriday.GoToNext
	}

	if len(node.Literal) > 0 {
		_, err := w.Write(node.Literal)
		if err != nil {
			panic(fmt.Errorf("error writing markdown to buffer: %w"))
		}
	}

	return blackfriday.GoToNext
}

func (rend *unwrapRenderer) renderPar(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		io.WriteString(w, "\n")
	}

	// If we're in a block quote, render the starting mark.
	if rend.quoteLevel > 0 {
		io.WriteString(w, "> ")
	}

	// TODO: this is jank, but I couldn't figure out how to put two lines between
	// paragraphs to make write.as happy without also adding a line after eg. the
	// paragraph in a list or heading, etc.
	// Is there any other type that should also have a newline after it?
	// Is there a more general way to do this that works regardless of what type
	// comes before the paragraph?
	if node.Prev != nil && node.Prev.Type == blackfriday.Paragraph {
		io.WriteString(w, "\n")
	}
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderSpan(wrap string, w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	io.WriteString(w, wrap)
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderHR(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return blackfriday.GoToNext
	}

	io.WriteString(w, "---\n")
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderCode(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	fmt.Fprintf(w, "`%s`", node.Literal)
	return blackfriday.GoToNext
}

func (*unwrapRenderer) renderCodeBlock(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	fmt.Fprintf(w, "```%s\n%s```\n", node.CodeBlockData.Info, node.Literal)
	return blackfriday.GoToNext
}
