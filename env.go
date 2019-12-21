// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"strconv"
)

func envOrDef(key, def string) string {
	e := os.Getenv(key)
	if e == "" {
		return def
	}
	return e
}

func intEnv(key string) int {
	e := os.Getenv(key)
	if e == "" {
		return 0
	}
	i, err := strconv.Atoi(e)
	if err != nil {
		return 0
	}
	return i
}
