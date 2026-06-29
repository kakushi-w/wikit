package wiki

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"wikit/internal/jsonx"
	"wikit/internal/sevenzip"
)

// ---- page metadata ----

func (w *WikiDot) pageMetaPath(name string) string {
	return filepath.Join(w.workDir, "meta", "pages", normalizeName(name)+".json")
}

func (w *WikiDot) readPageMetadata(name string) *PageMeta {
	data, err := os.ReadFile(w.pageMetaPath(name))
	if err != nil {
		return nil
	}
	v, err := jsonx.Decode(data)
	if err != nil {
		return nil
	}
	o, ok := v.(*jsonx.Object)
	if !ok {
		return nil
	}
	return readPageMeta(o)
}

func (w *WikiDot) pageMetadataExists(name string) bool {
	st, err := os.Stat(w.pageMetaPath(name))
	return err == nil && !st.IsDir()
}

func (w *WikiDot) writePageMetadata(name string, m *PageMeta) error {
	return writeJSON(w.pageMetaPath(name), m.object())
}

func (w *WikiDot) markPageRemoved(name string) {
	norm := normalizeName(name)
	_ = os.Remove(filepath.Join(w.workDir, "meta", "pages", norm+".json"))
	_ = os.Remove(filepath.Join(w.workDir, "pages", norm+".7z"))
	_ = os.RemoveAll(filepath.Join(w.workDir, "pages", norm))
	_ = os.RemoveAll(filepath.Join(w.workDir, "files", norm))
}

// ---- revisions ----

func (w *WikiDot) writeRevision(name string, rev int64, body string) error {
	dir := filepath.Join(w.workDir, "pages", normalizeName(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, strconv.FormatInt(rev, 10)+".txt"), []byte(body), 0o644)
}

// revisionList returns the revision numbers already on disk, from both the
// loose .txt files and the compressed .7z, matching the original.
func (w *WikiDot) revisionList(name string) []int64 {
	norm := normalizeName(name)
	seen := map[int64]bool{}
	var out []int64
	add := func(n int64) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}

	dir := filepath.Join(w.workDir, "pages", norm)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if n, ok := parseTxtName(e.Name()); ok {
				add(n)
			}
		}
	}
	zip := filepath.Join(w.workDir, "pages", norm+".7z")
	if _, err := os.Stat(zip); err == nil {
		if members, err := sevenzip.List(zip); err == nil {
			for _, m := range members {
				if n, ok := parseTxtName(m); ok {
					add(n)
				}
			}
		}
	}
	return out
}

func parseTxtName(name string) (int64, bool) {
	if !strings.HasSuffix(name, ".txt") {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSuffix(name, ".txt"), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// compressRevisions packs pages/<norm>/*.txt into pages/<norm>.7z and removes the
// loose files, exactly as the original does.
func (w *WikiDot) compressRevisions(norm string) error {
	dir := filepath.Join(w.workDir, "pages", norm)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var txts []string
	shouldBeEmpty := true
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") {
			shouldBeEmpty = false
			continue
		}
		if _, err := strconv.ParseInt(strings.TrimSuffix(name, ".txt"), 10, 64); err != nil {
			shouldBeEmpty = false
			continue
		}
		txts = append(txts, filepath.Join(dir, name))
	}

	if len(txts) != 0 {
		zip := filepath.Join(w.workDir, "pages", norm+".7z")
		removeZeroLength(zip)
		if err := sevenzip.Add(zip, filepath.Join(dir, "*.txt"), false); err != nil {
			return err
		}
		for _, t := range txts {
			_ = os.Remove(t)
		}
	}
	if shouldBeEmpty {
		_ = os.RemoveAll(dir)
	} else {
		w.logf("%s is not empty, not removing it.", dir)
	}
	return nil
}

// ---- sitemap ----

type sitemapEntry struct {
	Name   string
	Update *int64 // ms, nil when the sitemap had no lastmod
}

func (w *WikiDot) sitemapPath() string { return filepath.Join(w.workDir, "meta", "sitemap.json") }

func (w *WikiDot) writeSiteMap(entries []sitemapEntry) error {
	o := jsonx.NewObject()
	for _, e := range entries {
		if e.Update == nil {
			o.Set(e.Name, nil)
		} else {
			o.Set(e.Name, *e.Update)
		}
	}
	return writeJSON(w.sitemapPath(), o)
}

// readSiteMap returns the previous sitemap (name -> ms or nil), or nil if none.
func (w *WikiDot) readSiteMap() map[string]*int64 {
	data, err := os.ReadFile(w.sitemapPath())
	if err != nil {
		return nil
	}
	v, err := jsonx.Decode(data)
	if err != nil {
		return nil
	}
	o, ok := v.(*jsonx.Object)
	if !ok {
		return nil
	}
	out := map[string]*int64{}
	for _, k := range o.Keys() {
		val, _ := o.Get(k)
		if val == nil {
			out[k] = nil
		} else {
			n := asInt64(val)
			out[k] = &n
		}
	}
	return out
}

func (w *WikiDot) writeSiteMeta(m SiteMeta) error {
	return writeJSON(filepath.Join(w.workDir, "meta", "site.json"), m.object())
}

// ---- forum ----

func (w *WikiDot) readForumCategory(id int64) *LocalForumCategory {
	data, err := os.ReadFile(filepath.Join(w.workDir, "meta", "forum", "category", strconv.FormatInt(id, 10)+".json"))
	if err != nil {
		return nil
	}
	v, err := jsonx.Decode(data)
	if err != nil {
		return nil
	}
	o, ok := v.(*jsonx.Object)
	if !ok {
		return nil
	}
	return readForumCategory(o)
}

func (w *WikiDot) writeForumCategory(c *LocalForumCategory) error {
	return writeJSON(filepath.Join(w.workDir, "meta", "forum", "category", strconv.FormatInt(c.ID, 10)+".json"), c.object())
}

func (w *WikiDot) readForumThread(cat, thread int64) *LocalForumThread {
	data, err := os.ReadFile(filepath.Join(w.workDir, "meta", "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(thread, 10)+".json"))
	if err != nil {
		return nil
	}
	v, err := jsonx.Decode(data)
	if err != nil {
		return nil
	}
	o, ok := v.(*jsonx.Object)
	if !ok {
		return nil
	}
	return readForumThread(o)
}

func (w *WikiDot) writeForumThread(cat int64, t *LocalForumThread) error {
	return writeJSON(filepath.Join(w.workDir, "meta", "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(t.ID, 10)+".json"), t.object())
}

func (w *WikiDot) writePostRevision(cat, thread, post int64, rev string, value string) error {
	dir := filepath.Join(w.workDir, "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(thread, 10), strconv.FormatInt(post, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, rev+".html"), []byte(value), 0o644)
}

// readPostsAndRevisionsOfThread returns the set of post ids and per-post revision
// names already stored (loose and in the .7z).
func (w *WikiDot) readPostsAndRevisionsOfThread(cat, thread int64) (map[int64]bool, map[int64][]string) {
	posts := map[int64]bool{}
	revs := map[int64][]string{}

	base := filepath.Join(w.workDir, "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(thread, 10))
	if postDirs, err := os.ReadDir(base); err == nil {
		for _, pd := range postDirs {
			pid, err := strconv.ParseInt(pd.Name(), 10, 64)
			if err != nil {
				continue
			}
			posts[pid] = true
			if files, err := os.ReadDir(filepath.Join(base, pd.Name())); err == nil {
				for _, f := range files {
					if strings.HasSuffix(f.Name(), ".html") {
						revs[pid] = append(revs[pid], strings.TrimSuffix(f.Name(), ".html"))
					}
				}
			}
		}
	}
	zip := filepath.Join(w.workDir, "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(thread, 10)+".7z")
	if _, err := os.Stat(zip); err == nil {
		if members, err := sevenzip.List(zip); err == nil {
			for _, m := range members {
				slash := strings.IndexByte(m, '/')
				if slash < 0 || !strings.HasSuffix(m, ".html") {
					continue
				}
				pid, err := strconv.ParseInt(m[:slash], 10, 64)
				if err != nil {
					continue
				}
				posts[pid] = true
				revs[pid] = append(revs[pid], strings.TrimSuffix(m[slash+1:], ".html"))
			}
		}
	}
	return posts, revs
}

// compressForumThread packs forum/<cat>/<thread>/*.* into <thread>.7z and removes
// the directory, mirroring the original's validation and call.
func (w *WikiDot) compressForumThread(cat, thread int64) error {
	dir := filepath.Join(w.workDir, "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(thread, 10))
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	zip := filepath.Join(w.workDir, "forum", strconv.FormatInt(cat, 10), strconv.FormatInt(thread, 10)+".7z")
	removeZeroLength(zip)
	if err := sevenzip.Add(zip, filepath.Join(dir, "*.*"), true); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func removeZeroLength(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() == 0 {
		_ = os.Remove(path)
	}
}
