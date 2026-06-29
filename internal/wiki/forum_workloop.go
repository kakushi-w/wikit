package wiki

import (
	"strconv"
	"sync"
	"time"

	"wikit/internal/httpc"
)

func (w *WikiDot) runForums() {
	w.logf("Fetching forums list...")
	forums, err := w.fetchForumCategories()
	if err != nil {
		if he, ok := err.(*httpc.HTTPError); ok && he.Status == 404 {
			w.errf("No forums present")
		} else {
			w.errf("Error while fetching forum list: %v, will not try again", err)
		}
		forums = nil
	}

	for _, forum := range forums {
		local := w.readForumCategory(forum.ID)
		if local != nil && nullIntEqual(local.Last, forum.Last) && local.FullScan &&
			local.Version != nil && *local.Version == forumCategoryMetadataVersion {
			continue
		}
		fullScan := local != nil && local.FullScan
		page := int64(0)
		if local != nil {
			page = local.LastPage
		}

		for {
			updated := false
			page++
			w.logf("Fetching threads of %d offset %d", forum.ID, page)
			threads, err := w.fetchThreads(forum.ID, int(page))
			if err != nil {
				w.errf("Error fetching threads of %d: %v", forum.ID, err)
				break
			}

			var mu sync.Mutex
			w.runThreadWorkers(forum, threads, &updated, &mu)

			if len(threads) == 0 || (!updated && fullScan) {
				_ = w.writeForumCategory(categoryMeta(forum, true, 0))
				break
			}
			_ = w.writeForumCategory(categoryMeta(forum, fullScan, page))
		}
	}
	w.logf("Fetched all forums!")
}

func (w *WikiDot) runThreadWorkers(forum remoteForumCategory, threads []remoteForumThread, updated *bool, mu *sync.Mutex) {
	ch := make(chan remoteForumThread)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for th := range ch {
				if w.processThread(forum, th) {
					mu.Lock()
					*updated = true
					mu.Unlock()
				}
			}
		}()
	}
	for _, th := range threads {
		ch <- th
	}
	close(ch)
	wg.Wait()
}

// processThread fetches a thread if its local copy is stale, returning whether it
// was updated.
func (w *WikiDot) processThread(forum remoteForumCategory, thread remoteForumThread) bool {
	local := w.readForumThread(forum.ID, thread.ID)
	shouldFetch := local == nil || !nullIntEqual(local.Last, thread.Last)

	var localPosts map[int64]bool
	var knownRevs map[int64][]string
	haveRead := false

	if !shouldFetch && local != nil {
		count := int64(0)
		for _, p := range local.Posts {
			count += countPosts(p)
		}
		shouldFetch = count != thread.PostsNum
		if shouldFetch {
			w.errf("Post amount mismatch of %d (expected %d, got %d)", thread.ID, thread.PostsNum, count)
		}
		if !shouldFetch {
			localPosts, knownRevs = w.readPostsAndRevisionsOfThread(forum.ID, thread.ID)
			haveRead = true
			if int64(len(localPosts)) != count {
				w.errf("Fetched post count mismatch of %d (expected %d, got %d)", thread.ID, count, len(localPosts))
				shouldFetch = true
			}
		}
		if !shouldFetch {
			shouldFetch = local.Version == nil || *local.Version < forumThreadMetadataVersion
		}
	}

	if !shouldFetch {
		return false
	}

	w.logf("Fetching thread meta of %s (%d)", thread.Title, thread.ID)
	var oldPosts []LocalForumPost
	if local != nil {
		oldPosts = local.Posts
	}
	if !haveRead {
		localPosts, knownRevs = w.readPostsAndRevisionsOfThread(forum.ID, thread.ID)
	}
	_ = localPosts

	version := int64(forumThreadMetadataVersion)
	newMeta := &LocalForumThread{
		Title:       thread.Title,
		ID:          thread.ID,
		Description: thread.Description,
		Last:        thread.Last,
		LastUser:    thread.LastUser,
		Started:     thread.Started,
		StartedUser: thread.StartedUser,
		PostsNum:    thread.PostsNum,
		Sticky:      thread.Sticky,
		IsLocked:    w.fetchIsThreadLockedForce(thread.ID),
		Version:     &version,
	}

	posts, err := w.fetchAllThreadPosts(thread.ID)
	if err != nil {
		w.errf("Error fetching posts of %d: %v", thread.ID, err)
		return false
	}

	fetchOnce := false
	for _, post := range posts {
		lp := w.workWithPost(forum.ID, thread.ID, post, oldPosts, knownRevs, &fetchOnce)
		newMeta.Posts = append(newMeta.Posts, lp)
	}

	_ = w.writeForumThread(forum.ID, newMeta)
	if fetchOnce {
		_ = w.compressForumThread(forum.ID, thread.ID)
	}
	return true
}

func (w *WikiDot) workWithPost(cat, thread int64, post parsedPost, oldPosts []LocalForumPost, knownRevs map[int64][]string, fetchOnce *bool) LocalForumPost {
	oldPost := findPost(oldPosts, post.ID)
	lp := LocalForumPost{
		ID:         post.ID,
		Title:      post.Title,
		Poster:     post.Poster,
		Stamp:      post.Stamp,
		LastEdit:   post.LastEdit,
		LastEditBy: post.LastEditBy,
	}
	if oldPost != nil {
		lp.Revisions = append(lp.Revisions, oldPost.Revisions...)
	}
	existing := knownRevs[post.ID]

	if post.LastEdit.Set {
		w.logf("Fetching revision list of post %d", post.ID)
		revList, err := w.fetchPostRevisionList(post.ID)
		fetched := false
		if err == nil {
			for _, r := range revList {
				if containsStr(existing, strconv.FormatInt(r.ID, 10)) && findPostRevision(lp.Revisions, r.ID) != nil {
					continue
				}
				content, title, err := w.fetchPostRevision(r.ID)
				if err != nil {
					if he, ok := err.(*httpc.HTTPError); ok && he.Status == 500 {
						continue
					}
					continue
				}
				_ = w.writePostRevision(cat, thread, post.ID, strconv.FormatInt(r.ID, 10), content)
				*fetchOnce = true
				fetched = true
				// replace any existing revision with the same id
				for i := range lp.Revisions {
					if lp.Revisions[i].ID == r.ID {
						lp.Revisions = append(lp.Revisions[:i], lp.Revisions[i+1:]...)
						break
					}
				}
				lp.Revisions = append(lp.Revisions, LocalPostRevision{
					Title:  title,
					Author: r.Author,
					ID:     r.ID,
					Stamp:  r.Stamp,
				})
			}
		}
		if fetched || !containsStr(existing, "latest") {
			_ = w.writePostRevision(cat, thread, post.ID, "latest", post.Content)
			*fetchOnce = true
		}
	} else if !containsStr(existing, "latest") {
		_ = w.writePostRevision(cat, thread, post.ID, "latest", post.Content)
		*fetchOnce = true
	}

	for _, child := range post.Children {
		lp.Children = append(lp.Children, w.workWithPost(cat, thread, child, oldPosts, knownRevs, fetchOnce))
	}
	return lp
}

func (w *WikiDot) fetchIsThreadLockedForce(threadID int64) bool {
	for i := 0; i < 3; i++ {
		locked, err := w.fetchIsThreadLocked(threadID)
		if err == nil {
			return locked
		}
		w.errf("Encountered %v while fetching lock status of thread %d, sleeping 2s (tries left: %d)", err, threadID, 3-i-1)
		time.Sleep(2 * time.Second)
	}
	return false
}

func categoryMeta(forum remoteForumCategory, fullScan bool, lastPage int64) *LocalForumCategory {
	version := int64(forumCategoryMetadataVersion)
	return &LocalForumCategory{
		Title:       forum.Title,
		Description: forum.Description,
		ID:          forum.ID,
		Last:        forum.Last,
		Posts:       forum.Posts,
		Threads:     forum.Threads,
		LastUser:    forum.LastUser,
		FullScan:    fullScan,
		LastPage:    lastPage,
		Version:     &version,
	}
}

func countPosts(p LocalForumPost) int64 {
	count := int64(1)
	for _, c := range p.Children {
		count += countPosts(c)
	}
	return count
}

func findPost(list []LocalForumPost, id int64) *LocalForumPost {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
		if r := findPost(list[i].Children, id); r != nil {
			return r
		}
	}
	return nil
}

func findPostRevision(list []LocalPostRevision, id int64) *LocalPostRevision {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func nullIntEqual(a, b NullInt) bool {
	if a.Set != b.Set {
		return false
	}
	if !a.Set {
		return true
	}
	if a.Null != b.Null {
		return false
	}
	if a.Null {
		return true
	}
	return a.Val == b.Val
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
