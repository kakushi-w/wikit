package wiki

import (
	"wikit/internal/jsonx"
)

// NullInt models a JSON integer field that may be absent, null, or a number —
// the three states the original distinguishes (an undefined property is omitted,
// a UserID may be null, a real id is a number).
type NullInt struct {
	Set  bool // false => omit the key entirely
	Null bool // true (with Set) => emit null
	Val  int64
}

func num(v int64) NullInt     { return NullInt{Set: true, Val: v} }
func nullUser() NullInt       { return NullInt{Set: true, Null: true} }
func absent() NullInt         { return NullInt{} }
func userID(v *int64) NullInt { // always emitted: null when nil
	if v == nil {
		return nullUser()
	}
	return num(*v)
}

// putInt appends a NullInt to o under key, honoring the absent/null/number states.
func putInt(o *jsonx.Object, key string, n NullInt) {
	if !n.Set {
		return
	}
	if n.Null {
		o.Set(key, nil)
		return
	}
	o.Set(key, n.Val)
}

func putStr(o *jsonx.Object, key string, s *string) {
	if s != nil {
		o.Set(key, *s)
	}
}

func putBool(o *jsonx.Object, key string, b *bool) {
	if b != nil {
		o.Set(key, *b)
	}
}

// ---- Site metadata ----

type SiteMeta struct {
	Domain   string
	SiteID   int64
	Slug     string
	HomePage string
	Language string
}

func (m SiteMeta) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("domain", m.Domain)
	o.Set("site_id", m.SiteID)
	o.Set("slug", m.Slug)
	o.Set("home_page", m.HomePage)
	o.Set("language", m.Language)
	return o
}

// ---- Page revisions ----

type PageRevision struct {
	Revision       int64
	GlobalRevision int64
	Author         NullInt // UserID: always emitted (null or number)
	Stamp          NullInt // optional
	Flags          *string // optional
	Commentary     *string // optional
}

func (r PageRevision) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("revision", r.Revision)
	o.Set("global_revision", r.GlobalRevision)
	putInt(o, "author", r.Author)
	putInt(o, "stamp", r.Stamp)
	putStr(o, "flags", r.Flags)
	putStr(o, "commentary", r.Commentary)
	return o
}

// ---- File metadata ----

type FileMeta struct {
	FileID          int64
	Name            string
	URL             string
	Size            string
	SizeBytes       int64
	Mime            string
	Content         string
	Author          NullInt // UserID
	Stamp           int64
	InternalVersion *int64 // optional
}

func (f FileMeta) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("file_id", f.FileID)
	o.Set("name", f.Name)
	o.Set("url", f.URL)
	o.Set("size", f.Size)
	o.Set("size_bytes", f.SizeBytes)
	o.Set("mime", f.Mime)
	o.Set("content", f.Content)
	putInt(o, "author", f.Author)
	o.Set("stamp", f.Stamp)
	if f.InternalVersion != nil {
		o.Set("internal_version", *f.InternalVersion)
	}
	return o
}

// ---- Page metadata ----

type PageMeta struct {
	Name          string
	Version       *int64
	Revisions     []PageRevision
	Files         []FileMeta
	PageID        int64
	Parent        *string
	Rating        *int64
	ForumThread   *int64
	Tags          *[]string
	Title         *string
	SitemapUpdate *int64
	Votings       *[]Voting
	IsLocked      *bool

	// fromExistingPath records whether this metadata was constructed via the
	// incremental-update branch (votings positioned right after page_id) versus
	// the fresh-page branch (votings near the end). It governs field order so
	// the written bytes match the original exactly.
	fromExistingPath bool
}

// Voting is a single [UserID, bool] rating entry.
type Voting struct {
	User  NullInt
	Value bool
}

func votingsArray(v []Voting) []any {
	arr := make([]any, 0, len(v))
	for _, e := range v {
		pair := make([]any, 0, 2)
		if e.User.Null || !e.User.Set {
			pair = append(pair, nil)
		} else {
			pair = append(pair, e.User.Val)
		}
		pair = append(pair, e.Value)
		arr = append(arr, pair)
	}
	return arr
}

func revisionsArray(rs []PageRevision) []any {
	arr := make([]any, 0, len(rs))
	for _, r := range rs {
		arr = append(arr, r.object())
	}
	return arr
}

func filesArray(fs []FileMeta) []any {
	arr := make([]any, 0, len(fs))
	for _, f := range fs {
		arr = append(arr, f.object())
	}
	return arr
}

func (m PageMeta) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("name", m.Name)
	if m.Version != nil {
		o.Set("version", *m.Version)
	}
	o.Set("revisions", revisionsArray(m.Revisions))
	o.Set("files", filesArray(m.Files))
	o.Set("page_id", m.PageID)

	if m.fromExistingPath {
		// literal {name, version, revisions, files, page_id, votings, parent}
		if m.Votings != nil {
			o.Set("votings", votingsArray(*m.Votings))
		}
		putStr(o, "parent", m.Parent)
		if m.Rating != nil {
			o.Set("rating", *m.Rating)
		}
		if m.ForumThread != nil {
			o.Set("forum_thread", *m.ForumThread)
		}
		if m.Tags != nil {
			o.Set("tags", stringsArray(*m.Tags))
		}
		putStr(o, "title", m.Title)
		if m.SitemapUpdate != nil {
			o.Set("sitemap_update", *m.SitemapUpdate)
		}
		putBool(o, "is_locked", m.IsLocked)
	} else {
		// literal {name, version, revisions, files, page_id, parent}; then
		// rating, forum_thread, tags, title, sitemap_update, votings, is_locked
		putStr(o, "parent", m.Parent)
		if m.Rating != nil {
			o.Set("rating", *m.Rating)
		}
		if m.ForumThread != nil {
			o.Set("forum_thread", *m.ForumThread)
		}
		if m.Tags != nil {
			o.Set("tags", stringsArray(*m.Tags))
		}
		putStr(o, "title", m.Title)
		if m.SitemapUpdate != nil {
			o.Set("sitemap_update", *m.SitemapUpdate)
		}
		if m.Votings != nil {
			o.Set("votings", votingsArray(*m.Votings))
		}
		putBool(o, "is_locked", m.IsLocked)
	}
	return o
}

func stringsArray(ss []string) []any {
	arr := make([]any, 0, len(ss))
	for _, s := range ss {
		arr = append(arr, s)
	}
	return arr
}

// ---- Forum metadata ----

type LocalForumCategory struct {
	Title       string
	Description string
	ID          int64
	Last        NullInt // optional
	Posts       int64
	Threads     int64
	LastUser    NullInt // UserID (always emitted)
	FullScan    bool
	LastPage    int64
	Version     *int64
}

func (c LocalForumCategory) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("title", c.Title)
	o.Set("description", c.Description)
	o.Set("id", c.ID)
	putInt(o, "last", c.Last)
	o.Set("posts", c.Posts)
	o.Set("threads", c.Threads)
	putInt(o, "lastUser", c.LastUser)
	o.Set("full_scan", c.FullScan)
	o.Set("last_page", c.LastPage)
	if c.Version != nil {
		o.Set("version", *c.Version)
	}
	return o
}

type LocalPostRevision struct {
	Title  string
	Author NullInt
	ID     int64
	Stamp  int64
}

func (r LocalPostRevision) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("title", r.Title)
	putInt(o, "author", r.Author)
	o.Set("id", r.ID)
	o.Set("stamp", r.Stamp)
	return o
}

type LocalForumPost struct {
	ID         int64
	Title      string
	Poster     NullInt // UserID
	Stamp      int64
	LastEdit   NullInt // optional
	LastEditBy NullInt // optional
	Revisions  []LocalPostRevision
	Children   []LocalForumPost
}

func (p LocalForumPost) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("id", p.ID)
	o.Set("title", p.Title)
	putInt(o, "poster", p.Poster)
	o.Set("stamp", p.Stamp)
	putInt(o, "lastEdit", p.LastEdit)
	putInt(o, "lastEditBy", p.LastEditBy)
	revs := make([]any, 0, len(p.Revisions))
	for _, r := range p.Revisions {
		revs = append(revs, r.object())
	}
	o.Set("revisions", revs)
	kids := make([]any, 0, len(p.Children))
	for _, c := range p.Children {
		kids = append(kids, c.object())
	}
	o.Set("children", kids)
	return o
}

type LocalForumThread struct {
	Title       string
	ID          int64
	Description string
	Last        NullInt // optional
	LastUser    NullInt // optional UserID
	Started     int64
	StartedUser NullInt // UserID (always emitted)
	PostsNum    int64
	Sticky      bool
	IsLocked    bool
	Version     *int64
	Posts       []LocalForumPost
}

func (t LocalForumThread) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("title", t.Title)
	o.Set("id", t.ID)
	o.Set("description", t.Description)
	putInt(o, "last", t.Last)
	putInt(o, "lastUser", t.LastUser)
	o.Set("started", t.Started)
	putInt(o, "startedUser", t.StartedUser)
	o.Set("postsNum", t.PostsNum)
	o.Set("sticky", t.Sticky)
	o.Set("isLocked", t.IsLocked)
	if t.Version != nil {
		o.Set("version", *t.Version)
	}
	posts := make([]any, 0, len(t.Posts))
	for _, p := range t.Posts {
		posts = append(posts, p.object())
	}
	o.Set("posts", posts)
	return o
}
