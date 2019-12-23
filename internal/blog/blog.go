// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

// Package blog contains helpers for manipulating site and blog source files.
package blog

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

// The header of supported metadata types.
const (
	HeaderTOML = "+++\n"
	HeaderYAML = "---\n"
)

// WalkPages walks the file tree rooted at root and calls walkFn for each page.
// It skips any files that do not end in the extension ".markdown" or ".md".
func WalkPages(root string, walkFn filepath.WalkFunc) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		switch {
		case ext != ".md" && ext != ".markdown":
			// Skip non-markdown files
			return nil
		case info.IsDir():
			// Decend into directories, but don't call walkFn for them.
			return nil
		}

		return walkFn(path, info, err)
	})
}

// Metadata contains parsed metadata including the type of the metadata in the
// file, and the offset of where the metadata ends.
type Metadata map[string]interface{}

// Decode extracts metadata from the provided page.
// It assumes the first byte is the metadata header.
//
// It supports decoding TOML wrapped in "+++\n" and YAML wrapped in "---\n"
// similar to Hugo or Jekyll.
func (m Metadata) Decode(f io.Reader) error {
	r := bufio.NewReader(f)

	header, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	metaBuf := new(bytes.Buffer)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	for line != header {
		_, err := metaBuf.WriteString(line)
		if err != nil {
			return err
		}

		line, err = r.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
	}

	switch header {
	case HeaderTOML:
		err = toml.Unmarshal(metaBuf.Bytes(), &m)
	case HeaderYAML:
		err = yaml.Unmarshal(metaBuf.Bytes(), m)
	}
	return err
}

// Has returns whether or not the key actually exists in the metadata.
func (m Metadata) get(key string) (interface{}, bool) {
	val, ok := m[key]
	return val, ok
}

// GetString parses the metadata value for key and returns it as a string.
// If the underlying value is not a valid string, an empty string is returned.
func (m Metadata) GetString(key string) string {
	val, ok := m.get(key)
	if !ok {
		return ""
	}

	ret, _ := val.(string)
	return ret
}

// GetBool parses the metadata value for key and returns it as a bool.
// If the underlying value is a string GetBool attempts to parse it using
// strconv.ParseBool.
func (m Metadata) GetBool(key string) bool {
	val, ok := m.get(key)
	if !ok {
		return false
	}

	switch v := val.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(v)
		return parsed
	}
	return false
}

var fmts = []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"}

// GetTime parses the metadata value for key and returns it as a timestamp.
// If the underlying value is not already a time.Time, or a string that can be
// parsed into a valid time, a nil value is returned.
func (m Metadata) GetTime(key string) *time.Time {
	val, ok := m.get(key)
	if !ok {
		return nil
	}

	switch t := val.(type) {
	case time.Time:
		return &t
	case string:
		for _, timeFmt := range fmts {
			out, err := time.Parse(timeFmt, t)
			if err == nil {
				return &out
			}
		}
	}
	return nil
}
