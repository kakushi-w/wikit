package wiki

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wikit/internal/httpc"
)

// WorkLoop performs a full incremental archive of the wiki. sitemapLock
// serializes the sitemap-fetch phase across concurrently-running wikis, matching
// the original's global lock.
func (w *WikiDot) WorkLoop(sitemapLock *sync.Mutex) error {
	w.initialize()

	// Site metadata.
	sitemapLock.Lock()
	w.logf("Fetching sitemap")
	siteMeta, err := w.fetchSiteMetadata()
	if err != nil {
		sitemapLock.Unlock()
		return err
	}
	if err := w.writeSiteMeta(siteMeta); err != nil {
		sitemapLock.Unlock()
		return err
	}
	var entries []sitemapEntry
	smErr := w.fetchSiteMap(w.url+"/sitemap.xml", &entries)
	sitemapLock.Unlock()
	if smErr != nil {
		return smErr
	}
	w.logf("Counting total %d pages", len(entries))

	oldMap := w.readSiteMap()
	if oldMap == nil {
		w.logf("No previous sitemap was found, doing full scan")
	} else {
		w.logf("Previous sitemap contains %d pages", len(oldMap))
		index := make(map[string]bool, len(entries))
		for _, e := range entries {
			index[e.Name] = true
		}
		for name := range oldMap {
			if !index[name] {
				w.logf("Page %s was removed", name)
				meta := w.readPageMetadata(name)
				w.markPageRemoved(name)
				if meta != nil {
					w.state.deletePageID(meta.PageID)
				}
			}
		}
	}

	// Process pages.
	w.runPages(entries, oldMap)
	w.logf("Writing new sitemap...")
	if err := w.writeSiteMap(entries); err != nil {
		return err
	}

	// Forums.
	w.runForums()

	// Pending files and revisions.
	w.processPendingFiles()
	w.processPendingRevisions()

	// Final compression sweep of any leftover loose directories.
	w.compressLeftovers()

	return w.state.flush()
}

func (w *WikiDot) runPages(entries []sitemapEntry, oldMap map[string]*int64) {
	w.logf("Fetching changed pages...")
	tasks := make(chan sitemapEntry)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range tasks {
				w.processPage(e, oldMap)
			}
		}()
	}
	for _, e := range entries {
		tasks <- e
	}
	close(tasks)
	wg.Wait()
}

func (w *WikiDot) processPage(entry sitemapEntry, oldMap map[string]*int64) {
	name := entry.Name
	pageUpdate := entry.Update

	if oldMap != nil {
		if oldStamp, ok := oldMap[name]; ok {
			if stampEqual(oldStamp, pageUpdate) {
				if w.ultraFast || w.pageMetadataExists(name) {
					return
				}
			}
		}
	}

	metadata := w.readPageMetadata(name)
	needRenew := metadata == nil ||
		pageUpdate == nil ||
		metadata.SitemapUpdate == nil || *metadata.SitemapUpdate != *pageUpdate ||
		metadata.PageID == 0 ||
		metadata.Version == nil || *metadata.Version < pageMetadataVersion

	if needRenew {
		w.logf("Need to renew %s", name)
		pageMeta, err := w.fetchGeneric(name)
		if err != nil {
			w.logf("Encountered %v, postponing page %s for late fetch", err, name)
			w.state.pushPendingPage(name)
			return
		}
		if pageMeta.PageID != nil {
			metadata = w.buildAndRenew(name, pageUpdate, metadata, pageMeta)
		}
	}

	if metadata == nil || metadata.PageID == 0 {
		w.state.pushPendingPage(name)
		return
	}

	// Fetch any revisions not yet on disk.
	local := w.revisionList(name)
	localSet := make(map[int64]bool, len(local))
	for _, r := range local {
		localSet[r] = true
	}
	var toFetch []PageRevision
	for _, r := range metadata.Revisions {
		if !localSet[r.Revision] {
			toFetch = append(toFetch, r)
		}
	}

	var changed bool
	var mu sync.Mutex
	var rwg sync.WaitGroup
	revCh := make(chan PageRevision)
	for i := 0; i < 8; i++ {
		rwg.Add(1)
		go func() {
			defer rwg.Done()
			for rev := range revCh {
				for attempt := 0; attempt < 2; attempt++ {
					w.logf("Fetching revision %d (%d) of %s", rev.Revision, rev.GlobalRevision, name)
					body, err := w.fetchRevision(rev.GlobalRevision)
					if err != nil {
						w.errf("Encountered %v, postponing revision %d of %s", err, rev.GlobalRevision, name)
						w.state.addPendingRevision(rev.GlobalRevision, metadata.PageID)
						continue
					}
					if err := w.writeRevision(name, rev.Revision, body); err == nil {
						mu.Lock()
						changed = true
						mu.Unlock()
						w.state.deletePendingRevision(rev.GlobalRevision)
					}
					w.delay()
					break
				}
			}
		}()
	}
	for _, rev := range toFetch {
		revCh <- rev
	}
	close(revCh)
	rwg.Wait()

	w.state.removePendingPage(name)
	_ = w.writePageMetadata(name, metadata)
	if changed {
		_ = w.compressRevisions(normalizeName(name))
	}
}

// buildAndRenew reconstructs page metadata via the appropriate branch and fetches
// voters, files, lock status and new revisions, mirroring the original exactly so
// the written field order matches.
func (w *WikiDot) buildAndRenew(name string, pageUpdate *int64, metadata *PageMeta, pageMeta *GenericPageData) *PageMeta {
	version := int64(pageMetadataVersion)
	var newMeta *PageMeta

	if metadata == nil || (metadata.PageID != -1 && metadata.PageID != *pageMeta.PageID) {
		newMeta = &PageMeta{
			Name:      name,
			Version:   &version,
			Revisions: []PageRevision{},
			Files:     []FileMeta{},
			PageID:    *pageMeta.PageID,
			Parent:    pageMeta.Parent,
		}
		if metadata != nil {
			w.logf("Page %s got replaced", name)
			w.markPageRemoved(name)
			w.state.deletePageID(metadata.PageID)
		}
	} else {
		newMeta = &PageMeta{
			Name:             name,
			Version:          &version,
			Revisions:        metadata.Revisions,
			Files:            metadata.Files,
			PageID:           metadata.PageID,
			Votings:          metadata.Votings,
			Parent:           pageMeta.Parent,
			fromExistingPath: true,
		}
	}
	w.state.setPageID(newMeta.PageID, name)

	newMeta.Rating = pageMeta.Rating
	newMeta.ForumThread = pageMeta.ForumThread
	if len(pageMeta.Tags) > 0 {
		tags := pageMeta.Tags
		newMeta.Tags = &tags
	} else {
		newMeta.Tags = nil
	}
	newMeta.Title = pageMeta.PageName
	if pageUpdate != nil {
		newMeta.SitemapUpdate = pageUpdate
	}

	for i := 0; i < 3; i++ {
		if v, err := w.fetchPageVoters(newMeta.PageID); err == nil {
			newMeta.Votings = v
			break
		} else {
			w.errf("Encountered error fetching %s voters: %v", name, err)
		}
	}

	for i := 0; i < 3; i++ {
		oldFiles := newMeta.Files
		nf := w.fetchFilesFor(newMeta.PageID, newMeta.Files)
		newMeta.Files = nf
		for _, of := range oldFiles {
			found := false
			for _, nfm := range nf {
				if nfm.FileID == of.FileID {
					found = true
					break
				}
			}
			if !found {
				w.logf("File %d <%s> inside %s got removed", of.FileID, of.URL, name)
				_ = os.Remove(filepath.Join(w.workDir, "files", name, strconv.FormatInt(of.FileID, 10)))
			}
		}
		break
	}

	for i := 0; i < 3; i++ {
		if l, err := w.fetchIsPageLocked(newMeta.PageID); err == nil {
			newMeta.IsLocked = &l
			break
		} else {
			w.errf("Encountered error fetching %s \"is locked\" status: %v", name, err)
		}
	}

	lastRev, has := findMostRevision(newMeta.Revisions)
	var changes []PageRevision
	if !has {
		changes, _ = w.fetchPageChangeListAll(newMeta.PageID)
	} else {
		changes, _ = w.fetchPageChangeListAllUntil(newMeta.PageID, lastRev)
	}
	newMeta.Revisions = append(changes, newMeta.Revisions...)

	_ = w.writePageMetadata(name, newMeta)
	return newMeta
}

func (w *WikiDot) processPendingFiles() {
	w.state.mu.Lock()
	pending := append([]int64(nil), w.state.pendingFiles...)
	w.state.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	w.logf("Fetching pending files")
	for i := len(pending) - 1; i >= 0; i-- {
		id := pending[i]
		w.state.mu.Lock()
		mapped, ok := w.state.fileMap[id]
		w.state.mu.Unlock()
		if !ok {
			w.logf("Re-fetching file meta of %d", id)
			fm, err := w.fetchFileMeta(id)
			if err != nil {
				continue
			}
			if fm == nil {
				w.state.removePendingFile(id)
				continue
			}
			pageName, fileName, _, ok := splitFilePathRaw(fm.URL)
			if !ok {
				continue
			}
			w.state.setFileMap(id, fileMapEntry{URL: fm.URL, Path: pageName + "/" + fileName})
			mapped = fileMapEntry{URL: fm.URL, Path: pageName + "/" + fileName}
		}
		pageName, _, _, ok := splitFilePathRaw(mapped.URL)
		if !ok {
			continue
		}
		if w.fileExists(pageName, id, 0) {
			continue
		}
		w.fetchFileInner(mapped.URL, id, pageName)
	}
}

func (w *WikiDot) processPendingRevisions() {
	w.state.mu.Lock()
	type pr struct{ global, page int64 }
	var copy []pr
	for g, p := range w.state.pendingRevisions {
		copy = append(copy, pr{g, p})
	}
	w.state.mu.Unlock()
	if len(copy) == 0 {
		return
	}
	w.logf("Fetching pending revisions")
	for _, c := range copy {
		w.state.mu.Lock()
		name := w.state.pageIDMap[c.page]
		w.state.mu.Unlock()
		if name == "" {
			w.state.deletePendingRevision(c.global)
			continue
		}
		meta := w.readPageMetadata(name)
		if meta == nil {
			w.state.deletePendingRevision(c.global)
			continue
		}
		var rev *PageRevision
		for i := range meta.Revisions {
			if meta.Revisions[i].GlobalRevision == c.global {
				rev = &meta.Revisions[i]
				break
			}
		}
		if rev == nil {
			w.state.deletePendingRevision(c.global)
			continue
		}
		body, err := w.fetchRevision(rev.GlobalRevision)
		if err != nil {
			if strings.HasPrefix(name, "nav:") || strings.HasPrefix(name, "tech:") {
				w.state.deletePendingRevision(c.global)
			}
			continue
		}
		_ = w.writeRevision(name, rev.Revision, body)
		w.state.deletePendingRevision(c.global)
		_ = w.compressRevisions(normalizeName(name))
	}
}

// compressLeftovers compresses any remaining loose page/forum directories, as the
// original does at the end of the run.
func (w *WikiDot) compressLeftovers() {
	w.logf("Compressing page revisions")
	pagesDir := filepath.Join(w.workDir, "pages")
	if entries, err := os.ReadDir(pagesDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() && !strings.HasPrefix(name, ".") {
				_ = w.compressRevisions(name)
			}
		}
	}
	forumDir := filepath.Join(w.workDir, "forum")
	if cats, err := os.ReadDir(forumDir); err == nil {
		w.logf("Compressing forum threads")
		for _, cat := range cats {
			if !cat.IsDir() || strings.HasPrefix(cat.Name(), ".") {
				continue
			}
			catID, err := strconv.ParseInt(cat.Name(), 10, 64)
			if err != nil {
				continue
			}
			threads, err := os.ReadDir(filepath.Join(forumDir, cat.Name()))
			if err != nil {
				continue
			}
			for _, th := range threads {
				if th.IsDir() && !strings.HasPrefix(th.Name(), ".") {
					if thID, err := strconv.ParseInt(th.Name(), 10, 64); err == nil {
						_ = w.compressForumThread(catID, thID)
					}
				}
			}
		}
	}
}

func stampEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

var _ = httpc.HTTPError{}
var _ = time.Second
