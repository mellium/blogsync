// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"reflect"

	"github.com/writeas/go-writeas/v2"
)

// eqPost returns whether two posts are equal for the purpose of updating them.
func eqPost(p1, p2 *writeas.Post) bool {
	switch {
	case p1 == nil && p2 == nil:
		return true
	case p1 == nil || p2 == nil:
		return false
	}

	switch {
	case p1.ID != p2.ID:
		return false
	case p1.Slug != p2.Slug:
		return false
	case p1.Font != p2.Font:
		return false
	case !reflect.DeepEqual(p1.Language, p2.Language):
		return false
	case !reflect.DeepEqual(p1.RTL, p2.RTL):
		return false
	case p1.Title != p2.Title:
		return false
	case p1.Content != p2.Content:
		return false
		// TODO: compare tags when we support them.
	}

	return true
}
