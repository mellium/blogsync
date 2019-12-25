// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"time"

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

func eqParams(p *writeas.Post, params *writeas.PostParams) bool {
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
		Views:    p.Views,
		// TODO: what is this?
		Listed: p.Listed,
		// TODO: implement tags
		Tags:      p.Tags,
		Images:    p.Images,
		OwnerName: p.OwnerName,
		// TODO: implement collection changing if ID is set on the post
		Collection: p.Collection,
	}
	return eqPost(&cmpPost, p)
}
