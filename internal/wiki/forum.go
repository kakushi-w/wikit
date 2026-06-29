package wiki

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	"wikit/internal/htmlx"
)

var (
	reCategory = regexp.MustCompile(`forum/c-([0-9]+)`)
	rePost     = regexp.MustCompile(`post-([0-9]+)`)
)

// remoteForumCategory is a category as listed on the forum index.
type remoteForumCategory struct {
	Title       string
	Description string
	ID          int64
	Last        NullInt
	Posts       int64
	Threads     int64
	LastUser    NullInt
}

// remoteForumThread is a thread as listed in a category.
type remoteForumThread struct {
	Title       string
	ID          int64
	Description string
	Last        NullInt
	LastUser    NullInt
	Started     int64
	StartedUser NullInt
	PostsNum    int64
	Sticky      bool
}

// parsedPost is a forum post parsed from the thread view (content is innerHTML).
type parsedPost struct {
	ID         int64
	Title      string
	Poster     NullInt
	Content    string
	Stamp      int64
	LastEdit   NullInt
	LastEditBy NullInt
	Children   []parsedPost
}

type postRevisionInfo struct {
	Author NullInt
	Stamp  int64
	ID     int64
}

func (w *WikiDot) fetchForumCategories() ([]remoteForumCategory, error) {
	body, err := w.get(w.url+"/forum/start/hidden/show", nil)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	var listing []remoteForumCategory
	forum := doc.Find("div.forum-start-box").First()
	if forum.Length() == 0 {
		return listing, nil
	}
	forum.Find("table").Each(func(_ int, table *goquery.Selection) {
		rows := table.Find("tr")
		rows.Each(func(_ int, row *goquery.Selection) {
			if cls, _ := row.Attr("class"); cls == "head" {
				return
			}
			name := row.Find("td.name").First()
			threads := row.Find("td.threads").First()
			posts := row.Find("td.posts").First()
			last := row.Find("td.last").First()
			if name.Length() == 0 || threads.Length() == 0 || posts.Length() == 0 || last.Length() == 0 {
				return
			}
			titleElem := name.Find("div.title a").First()
			href, _ := titleElem.Attr("href")
			m := reCategory.FindStringSubmatch(href)
			if m == nil {
				return
			}
			var lastDate NullInt
			if cls, ok := last.Find("span.odate").First().Attr("class"); ok {
				if dm := reDateClass.FindStringSubmatch(cls); dm != nil {
					lastDate = num(mustI64(dm[1]))
				}
			}
			listing = append(listing, remoteForumCategory{
				Title:       strings.TrimSpace(titleElem.Text()),
				Description: strings.TrimSpace(name.Find("div.description").First().Text()),
				ID:          mustI64(m[1]),
				Threads:     parseIntDefault(strings.TrimSpace(threads.Text())),
				Posts:       parseIntDefault(strings.TrimSpace(posts.Text())),
				Last:        lastDate,
				LastUser:    w.matchAndFetchUser(last),
			})
		})
	})
	return listing, nil
}

func (w *WikiDot) fetchThreads(category int64, page int) ([]remoteForumThread, error) {
	body, err := w.get(w.url+"/forum/c-"+strconv.FormatInt(category, 10)+"/p/"+strconv.Itoa(page), nil)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	var listing []remoteForumThread
	table := doc.Find("div#page-content table.table").First()
	if table.Length() == 0 {
		return listing, nil
	}
	table.Find("tr").Each(func(_ int, row *goquery.Selection) {
		if cls, _ := row.Attr("class"); cls == "head" {
			return
		}
		name := row.Find("td.name").First()
		started := row.Find("td.started").First()
		posts := row.Find("td.posts").First()
		last := row.Find("td.last").First()
		if name.Length() == 0 || started.Length() == 0 || posts.Length() == 0 || last.Length() == 0 {
			return
		}
		title := name.Find("div.title").First()
		titleElem := title.Find("a").First()
		href, _ := titleElem.Attr("href")
		m := reThread.FindStringSubmatch(href)
		if m == nil {
			return
		}
		sticky := false
		if len(title.Nodes) > 0 {
			if fc := title.Nodes[0].FirstChild; fc != nil && fc.Type == html.TextNode {
				sticky = strings.TrimSpace(fc.Data) != ""
			}
		}
		var lastDate, lastUser NullInt
		if last.Contents().Length() > 1 {
			if cls, ok := last.Find("span.odate").First().Attr("class"); ok {
				if dm := reDateClass.FindStringSubmatch(cls); dm != nil {
					lastDate = num(mustI64(dm[1]))
				}
			}
			lastUser = w.matchAndFetchUser(last)
		}
		var startedDate int64
		if cls, ok := started.Find("span.odate").First().Attr("class"); ok {
			if dm := reDateClass.FindStringSubmatch(cls); dm != nil {
				startedDate = mustI64(dm[1])
			}
		}
		listing = append(listing, remoteForumThread{
			Title:       strings.TrimSpace(titleElem.Text()),
			ID:          mustI64(m[1]),
			Description: strings.TrimSpace(name.Find("div.description").First().Text()),
			PostsNum:    parseIntDefault(strings.TrimSpace(posts.Text())),
			Sticky:      sticky,
			Last:        lastDate,
			LastUser:    lastUser,
			Started:     startedDate,
			StartedUser: w.matchAndFetchUser(started),
		})
	})
	return listing, nil
}

func (w *WikiDot) fetchIsThreadLocked(threadID int64) (bool, error) {
	env, err := w.ajaxJSON(map[string]string{
		"moduleName": "forum/sub/ForumNewPostFormModule",
		"threadId":   strconv.FormatInt(threadID, 10),
		"postId":     "",
	}, nil, true)
	if err != nil {
		return false, err
	}
	if env.Status == "ok" {
		return false, nil
	}
	doc, err := htmlx.Parse(env.Message)
	if err != nil {
		return false, err
	}
	return doc.Find("a").Length() == 0, nil
}

func (w *WikiDot) fetchThreadPosts(thread, page int64) ([]parsedPost, error) {
	env, err := w.ajaxJSON(map[string]string{
		"t":          strconv.FormatInt(thread, 10),
		"pageNo":     strconv.FormatInt(page, 10),
		"order":      "",
		"moduleName": "forum/ForumViewThreadPostsModule",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	doc, err := htmlx.Parse(env.Body)
	if err != nil {
		return nil, err
	}
	var posts []parsedPost
	// top-level post-containers are direct children of the document body
	doc.Find("body").Children().Each(func(_ int, el *goquery.Selection) {
		if cls, _ := el.Attr("class"); cls == "post-container" {
			posts = append(posts, w.parsePost(el, env.Body))
		}
	})
	// Fallback: some responses are not wrapped in body.
	if len(posts) == 0 {
		doc.Find("div.post-container").Each(func(_ int, el *goquery.Selection) {
			// only top-level ones (no post-container ancestor)
			if el.ParentsFiltered("div.post-container").Length() == 0 {
				posts = append(posts, w.parsePost(el, env.Body))
			}
		})
	}
	return posts, nil
}

func (w *WikiDot) fetchAllThreadPosts(thread int64) ([]parsedPost, error) {
	var listing []parsedPost
	for page := int64(1); ; page++ {
		w.logf("Fetching posts of %d offset %d", thread, page)
		got, err := w.fetchThreadPosts(thread, page)
		if err != nil {
			return nil, err
		}
		listing = append(listing, got...)
		if len(got) == 0 {
			break
		}
	}
	return listing, nil
}

func (w *WikiDot) parsePost(container *goquery.Selection, rawBody string) parsedPost {
	var post *goquery.Selection
	var childContainers []*goquery.Selection
	container.Children().Each(func(_ int, el *goquery.Selection) {
		cls, _ := el.Attr("class")
		switch cls {
		case "post":
			post = el
		case "post-container":
			childContainers = append(childContainers, el)
		}
	})
	if post == nil {
		return parsedPost{ID: -1, Title: "ERROR", Content: "ERROR", Stamp: -1, Poster: nullUser()}
	}
	idAttr, _ := post.Attr("id")
	m := rePost.FindStringSubmatch(idAttr)
	if m == nil {
		return parsedPost{ID: -1, Title: "ERROR", Content: "ERROR", Stamp: -1, Poster: nullUser()}
	}
	postID := mustI64(m[1])

	head := post.Find("div.head").First()
	title := post.Find("div.title").First()
	info := head.Find("div.info").First()

	var stamp int64
	if cls, ok := info.Find("span.odate").First().Attr("class"); ok {
		if dm := reDateClass.FindStringSubmatch(cls); dm != nil {
			stamp = mustI64(dm[1])
		}
	}
	content, _ := htmlx.ForumContentInnerHTML(rawBody, int(postID))

	p := parsedPost{
		ID:      postID,
		Title:   strings.TrimSpace(title.Text()),
		Poster:  w.matchAndFetchUser(info),
		Content: content,
		Stamp:   stamp,
	}

	if changes := post.Find("div.changes").First(); changes.Length() > 0 {
		if cls, ok := changes.Find("span.odate").First().Attr("class"); ok {
			if dm := reDateClass.FindStringSubmatch(cls); dm != nil {
				p.LastEdit = num(mustI64(dm[1]))
			}
		}
		p.LastEditBy = w.matchAndFetchUser(changes)
	}

	for _, child := range childContainers {
		p.Children = append(p.Children, w.parsePost(child, rawBody))
	}
	return p
}

func (w *WikiDot) fetchPostRevisionList(post int64) ([]postRevisionInfo, error) {
	env, err := w.ajaxJSON(map[string]string{
		"postId":     strconv.FormatInt(post, 10),
		"moduleName": "forum/sub/ForumPostRevisionsModule",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	doc, err := htmlx.Parse(env.Body)
	if err != nil {
		return nil, err
	}
	var listing []postRevisionInfo
	doc.Find("table.table tr").Each(func(_ int, row *goquery.Selection) {
		cols := row.Find("td")
		if cols.Length() < 3 {
			return
		}
		var stamp int64
		if cls, ok := cols.Eq(1).Find("span.odate").First().Attr("class"); ok {
			if dm := reDateClass.FindStringSubmatch(cls); dm != nil {
				stamp = mustI64(dm[1])
			}
		}
		oc, _ := cols.Eq(2).Find("a").First().Attr("onclick")
		idm := reDigits.FindStringSubmatch(oc)
		if idm == nil {
			return
		}
		listing = append(listing, postRevisionInfo{
			Author: w.matchAndFetchUser(cols.Eq(0)),
			Stamp:  stamp,
			ID:     mustI64(idm[1]),
		})
	})
	return listing, nil
}

func (w *WikiDot) fetchPostRevision(revisionID int64) (content string, title string, err error) {
	env, e := w.ajaxJSON(map[string]string{
		"revisionId": strconv.FormatInt(revisionID, 10),
		"moduleName": "forum/sub/ForumPostRevisionModule",
	}, nil, true)
	if e != nil {
		return "", "", e
	}
	return env.Content, env.Title, nil
}

func parseIntDefault(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
