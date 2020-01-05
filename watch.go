// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

func newWatcher(content string, debug *log.Logger) (watcher *fsnotify.Watcher, err error) {
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// Handle errors by cleaning up so that the caller can follow Go idioms of
	// checking errors before handling the value (eg. in case an error happens
	// while adding files to the already existing watcher).
	defer func() {
		if err != nil {
			if err := watcher.Close(); err != nil {
				debug.Printf("error closing unused %s watcher: %v", content, err)
			}
		}
	}()

	err = filepath.Walk(content, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			debug.Printf("error watching file %s, changes will not trigger a rebuilt: %v", path, err)
			return nil
		}

		if !info.IsDir() {
			// Watch entire directory trees for changes, not individual files.
			return nil
		}

		return watcher.Add(path)
	})

	return watcher, err
}
