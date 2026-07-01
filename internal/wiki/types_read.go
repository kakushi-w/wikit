package wiki

import (
	"strconv"

	"wikit/internal/jsonx"
)

// These readers decode the jsonx value model (which preserves key order and
// number literals) back into the working structs. Field order is irrelevant on
// read; it only matters when writing.

func asObject(v any) (*jsonx.Object, bool) {
	o, ok := v.(*jsonx.Object)
	return o, ok
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case jsonx.Number:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// readNullInt interprets a field as the absent/null/number tri-state.
func readNullInt(o *jsonx.Object, key string) NullInt {
	v, ok := o.Get(key)
	if !ok {
		return absent()
	}
	if v == nil {
		return nullUser()
	}
	return num(asInt64(v))
}

func readOptInt(o *jsonx.Object, key string) *int64 {
	v, ok := o.Get(key)
	if !ok || v == nil {
		return nil
	}
	i := asInt64(v)
	return &i
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case jsonx.Number:
		f, _ := strconv.ParseFloat(string(n), 64)
		return f
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}

// readOptFloat reads a number that may be integer or decimal (e.g. a +/- score
// like 5, or a five-star average like 4.5), preserving fractional values.
func readOptFloat(o *jsonx.Object, key string) *float64 {
	v, ok := o.Get(key)
	if !ok || v == nil {
		return nil
	}
	f := asFloat(v)
	return &f
}

func readOptStr(o *jsonx.Object, key string) *string {
	v, ok := o.Get(key)
	if !ok || v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func readOptBool(o *jsonx.Object, key string) *bool {
	v, ok := o.Get(key)
	if !ok || v == nil {
		return nil
	}
	b := asBool(v)
	return &b
}

func readPageRevision(o *jsonx.Object) PageRevision {
	r := PageRevision{
		Revision:       asInt64(mustGet(o, "revision")),
		GlobalRevision: asInt64(mustGet(o, "global_revision")),
		Author:         readNullInt(o, "author"),
		Stamp:          readNullInt(o, "stamp"),
		Flags:          readOptStr(o, "flags"),
		Commentary:     readOptStr(o, "commentary"),
	}
	return r
}

func readFileMeta(o *jsonx.Object) FileMeta {
	return FileMeta{
		FileID:          asInt64(mustGet(o, "file_id")),
		Name:            getStr(o, "name"),
		URL:             getStr(o, "url"),
		Size:            getStr(o, "size"),
		SizeBytes:       asInt64(mustGet(o, "size_bytes")),
		Mime:            getStr(o, "mime"),
		Content:         getStr(o, "content"),
		Author:          readNullInt(o, "author"),
		Stamp:           asInt64(mustGet(o, "stamp")),
		InternalVersion: readOptInt(o, "internal_version"),
	}
}

func readPageMeta(o *jsonx.Object) *PageMeta {
	m := &PageMeta{
		Name:          getStr(o, "name"),
		Version:       readOptInt(o, "version"),
		PageID:        asInt64(mustGet(o, "page_id")),
		Parent:        readOptStr(o, "parent"),
		Rating:        readOptFloat(o, "rating"),
		ForumThread:   readOptInt(o, "forum_thread"),
		Title:         readOptStr(o, "title"),
		SitemapUpdate: readOptInt(o, "sitemap_update"),
		IsLocked:      readOptBool(o, "is_locked"),
	}
	if v, ok := o.Get("revisions"); ok {
		if arr, ok := v.([]any); ok {
			for _, e := range arr {
				if eo, ok := asObject(e); ok {
					m.Revisions = append(m.Revisions, readPageRevision(eo))
				}
			}
		}
	}
	if v, ok := o.Get("files"); ok {
		if arr, ok := v.([]any); ok {
			for _, e := range arr {
				if eo, ok := asObject(e); ok {
					m.Files = append(m.Files, readFileMeta(eo))
				}
			}
		}
	}
	if v, ok := o.Get("tags"); ok {
		if arr, ok := v.([]any); ok {
			tags := make([]string, 0, len(arr))
			for _, e := range arr {
				if s, ok := e.(string); ok {
					tags = append(tags, s)
				}
			}
			m.Tags = &tags
		}
	}
	if v, ok := o.Get("votings"); ok {
		if arr, ok := v.([]any); ok {
			votings := make([]Voting, 0, len(arr))
			for _, e := range arr {
				pair, ok := e.([]any)
				if !ok || len(pair) != 2 {
					continue
				}
				vt := Voting{Value: asBool(pair[1])}
				if pair[0] == nil {
					vt.User = nullUser()
				} else {
					vt.User = num(asInt64(pair[0]))
				}
				votings = append(votings, vt)
			}
			m.Votings = &votings
		}
	}
	m.fromExistingPath = detectExistingPath(o)
	return m
}

// detectExistingPath infers which construction branch produced an existing page
// meta from its key order: in the incremental branch "votings" precedes the
// rating/.../sitemap_update group; in the fresh branch it follows them.
func detectExistingPath(o *jsonx.Object) bool {
	keys := o.Keys()
	idx := map[string]int{}
	for i, k := range keys {
		idx[k] = i
	}
	vi, hasV := idx["votings"]
	if !hasV {
		return false // order identical either way when votings absent
	}
	maxLater := -1
	for _, k := range []string{"rating", "forum_thread", "tags", "title", "sitemap_update"} {
		if i, ok := idx[k]; ok && i > maxLater {
			maxLater = i
		}
	}
	return vi < maxLater
}

func readForumCategory(o *jsonx.Object) *LocalForumCategory {
	return &LocalForumCategory{
		Title:       getStr(o, "title"),
		Description: getStr(o, "description"),
		ID:          asInt64(mustGet(o, "id")),
		Last:        readNullInt(o, "last"),
		Posts:       asInt64(mustGet(o, "posts")),
		Threads:     asInt64(mustGet(o, "threads")),
		LastUser:    readNullInt(o, "lastUser"),
		FullScan:    asBool(mustGet(o, "full_scan")),
		LastPage:    asInt64(mustGet(o, "last_page")),
		Version:     readOptInt(o, "version"),
	}
}

func readForumThread(o *jsonx.Object) *LocalForumThread {
	t := &LocalForumThread{
		Title:       getStr(o, "title"),
		ID:          asInt64(mustGet(o, "id")),
		Description: getStr(o, "description"),
		Last:        readNullInt(o, "last"),
		LastUser:    readNullInt(o, "lastUser"),
		Started:     asInt64(mustGet(o, "started")),
		StartedUser: readNullInt(o, "startedUser"),
		PostsNum:    asInt64(mustGet(o, "postsNum")),
		Sticky:      asBool(mustGet(o, "sticky")),
		IsLocked:    asBool(mustGet(o, "isLocked")),
		Version:     readOptInt(o, "version"),
	}
	if v, ok := o.Get("posts"); ok {
		if arr, ok := v.([]any); ok {
			t.Posts = readPosts(arr)
		}
	}
	return t
}

func readPosts(arr []any) []LocalForumPost {
	var out []LocalForumPost
	for _, e := range arr {
		o, ok := asObject(e)
		if !ok {
			continue
		}
		p := LocalForumPost{
			ID:         asInt64(mustGet(o, "id")),
			Title:      getStr(o, "title"),
			Poster:     readNullInt(o, "poster"),
			Stamp:      asInt64(mustGet(o, "stamp")),
			LastEdit:   readNullInt(o, "lastEdit"),
			LastEditBy: readNullInt(o, "lastEditBy"),
		}
		if rv, ok := o.Get("revisions"); ok {
			if rarr, ok := rv.([]any); ok {
				for _, re := range rarr {
					if ro, ok := asObject(re); ok {
						p.Revisions = append(p.Revisions, LocalPostRevision{
							Title:  getStr(ro, "title"),
							Author: readNullInt(ro, "author"),
							ID:     asInt64(mustGet(ro, "id")),
							Stamp:  asInt64(mustGet(ro, "stamp")),
						})
					}
				}
			}
		}
		if cv, ok := o.Get("children"); ok {
			if carr, ok := cv.([]any); ok {
				p.Children = readPosts(carr)
			}
		}
		out = append(out, p)
	}
	return out
}

func mustGet(o *jsonx.Object, key string) any {
	v, _ := o.Get(key)
	return v
}

func getStr(o *jsonx.Object, key string) string {
	if v, ok := o.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
