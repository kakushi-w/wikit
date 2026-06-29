package wiki

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"wikit/internal/htmlx"
)

// GenericPageData is the metadata scraped from a rendered page (matches the
// original GenericPageData).
type GenericPageData struct {
	PageID      *int64
	PageName    *string
	Rating      *int64
	ForumThread *int64
	Tags        []string
	Parent      *string
}

var (
	rePageID     = regexp.MustCompile(`(?i)WIKIREQUEST\.info\.pageId\s*=\s*([0-9]+);`)
	reThread     = regexp.MustCompile(`forum/t-([0-9]+)`)
	reTagA       = regexp.MustCompile(`(?i)/tag/(\S+)#pages$`)
	reTagB       = regexp.MustCompile(`(?i)/tag/(\S+)$`)
	reDateClass  = regexp.MustCompile(`time_([0-9]+)`)
	reDigits     = regexp.MustCompile(`([0-9]+)`)
)

func (w *WikiDot) fetchGeneric(page string) (*GenericPageData, error) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	body, err := w.getNoRedirect(w.url+"/"+page+"/noredirect/true?_ts="+ts, map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:100.0) Gecko/20100101 Firefox/100.0",
	})
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	meta := &GenericPageData{}
	doc.Find("head script").Each(func(_ int, s *goquery.Selection) {
		if m := rePageID.FindStringSubmatch(s.Text()); m != nil {
			meta.PageID = parseI64Ptr(m[1])
		}
	})
	if rt := doc.Find("span.rate-points span.number").First(); rt.Length() > 0 {
		meta.Rating = parseIntPrefix(strings.TrimSpace(rt.Text()))
	}
	if href, ok := doc.Find("#discuss-button").First().Attr("href"); ok {
		if m := reThread.FindStringSubmatch(href); m != nil {
			meta.ForumThread = parseI64Ptr(m[1])
		}
	}
	doc.Find("div.page-tags span a").Each(func(_ int, a *goquery.Selection) {
		if href, ok := a.Attr("href"); ok {
			var m []string
			if m = reTagA.FindStringSubmatch(href); m == nil {
				m = reTagB.FindStringSubmatch(href)
			}
			if m != nil {
				meta.Tags = append(meta.Tags, decodeURIComponent(m[1]))
			}
		}
	})
	if pn := doc.Find("div#page-title").First(); pn.Length() > 0 {
		t := strings.TrimSpace(pn.Text())
		meta.PageName = &t
	}
	bc := doc.Find("div#main-content div#breadcrumbs a")
	if bc.Length() > 0 {
		if href, ok := bc.Eq(bc.Length() - 1).Attr("href"); ok && len(href) > 0 {
			p := href[1:] // drop leading '/'
			meta.Parent = &p
		}
	}
	return meta, nil
}

// fetchPageChangeList fetches one page of a page's revision history.
func (w *WikiDot) fetchPageChangeList(pageID int64, page int) ([]PageRevision, error) {
	env, err := w.ajaxJSON(map[string]string{
		"options":    `{"all": true}`,
		"page":       strconv.Itoa(page),
		"perpage":    strconv.Itoa(defaultPagination),
		"page_id":    strconv.FormatInt(pageID, 10),
		"moduleName": "history/PageRevisionListModule",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	doc, err := htmlx.Parse(env.Body)
	if err != nil {
		return nil, err
	}

	var out []PageRevision
	skip := true
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		if skip {
			skip = false
			return
		}
		tds := row.Find("td")
		if tds.Length() < 7 {
			return
		}
		revM := reDigits.FindStringSubmatch(tds.Eq(0).Text())
		if revM == nil {
			return
		}
		flags := strings.TrimSpace(tds.Eq(2).Text())
		globalM := reDigits.FindStringSubmatch(attrOr(tds.Eq(3).Find("a").First(), "onclick"))
		if globalM == nil {
			return
		}
		author := w.matchAndFetchUser(tds.Eq(4))
		var stamp NullInt
		if cls, ok := tds.Eq(5).Find("span").First().Attr("class"); ok {
			if m := reDateClass.FindStringSubmatch(cls); m != nil {
				stamp = num(mustI64(m[1]))
			}
		}
		commentary := strings.TrimSpace(tds.Eq(6).Text())

		rev := mustI64(revM[1])
		gr := mustI64(globalM[1])
		flagsNorm := whitespaceRun.ReplaceAllString(flags, " ")
		out = append(out, PageRevision{
			Revision:       rev,
			GlobalRevision: gr,
			Author:         author,
			Stamp:          stamp,
			Flags:          &flagsNorm,
			Commentary:     &commentary,
		})
	})
	return out, nil
}

var whitespaceRun = regexp.MustCompile(`\s+`)

// fetchPageChangeListAll fetches the entire revision history.
func (w *WikiDot) fetchPageChangeListAll(pageID int64) ([]PageRevision, error) {
	var listing []PageRevision
	for page := 1; ; page++ {
		w.logf("Fetching changeset offset %d of %d", page-1, pageID)
		data, err := w.fetchPageChangeListForce(pageID, page)
		if err != nil {
			return nil, err
		}
		listing = append(listing, data...)
		if len(data) == 0 {
			break
		}
	}
	return listing, nil
}

// fetchPageChangeListAllUntil fetches revisions newer than the given revision.
func (w *WikiDot) fetchPageChangeListAllUntil(pageID, until int64) ([]PageRevision, error) {
	var listing []PageRevision
	for page := 1; ; page++ {
		w.logf("Fetching changeset offset %d of %d", page-1, pageID)
		data, err := w.fetchPageChangeListForce(pageID, page)
		if err != nil {
			return nil, err
		}
		finish := false
		for _, piece := range data {
			if piece.Revision <= until {
				finish = true
				break
			}
			listing = append(listing, piece)
		}
		if len(data) == 0 || finish {
			break
		}
	}
	return listing, nil
}

func (w *WikiDot) fetchPageChangeListForce(pageID int64, page int) ([]PageRevision, error) {
	for {
		data, err := w.fetchPageChangeList(pageID, page)
		if err == nil {
			return data, nil
		}
		w.errf("Encountered %v when fetching changes of %d offset %d, sleeping for 2 seconds", err, pageID, page)
		time.Sleep(2 * time.Second)
	}
}

// fetchRevision returns the wikitext source of a revision.
func (w *WikiDot) fetchRevision(globalRevision int64) (string, error) {
	env, err := w.ajaxJSON(map[string]string{
		"revision_id": strconv.FormatInt(globalRevision, 10),
		"moduleName":  "history/PageSourceModule",
	}, nil, false)
	if err != nil {
		return "", err
	}
	doc, err := htmlx.Parse(env.Body)
	if err != nil {
		return "", err
	}
	return htmlx.PageSourceText(doc), nil
}

// fetchPageVoters returns the rating list, or nil when absent.
func (w *WikiDot) fetchPageVoters(pageID int64) (*[]Voting, error) {
	env, err := w.ajaxJSON(map[string]string{
		"pageId":     strconv.FormatInt(pageID, 10),
		"moduleName": "pagerate/WhoRatedPageModule",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	doc, err := htmlx.Parse(env.Body)
	if err != nil {
		return nil, err
	}
	div := doc.Find("div").First()
	if div.Length() == 0 {
		return nil, nil
	}

	var listing []Voting
	var lastUser *NullInt
	var lastRating *bool
	var parseErr error
	div.Children().EachWithBreak(func(_ int, el *goquery.Selection) bool {
		tag := goquery.NodeName(el)
		switch tag {
		case "br":
			if lastUser == nil || lastRating == nil {
				parseErr = errMalformedVoting
				return false
			}
			listing = append(listing, Voting{User: *lastUser, Value: *lastRating})
			lastUser = nil
			lastRating = nil
		case "span":
			if lastUser == nil {
				u := w.matchAndFetchUser(el)
				lastUser = &u
			} else if lastRating == nil {
				decoded := strings.TrimSpace(el.Text())
				switch decoded {
				case "+":
					t := true
					lastRating = &t
				case "-":
					f := false
					lastRating = &f
				default:
					parseErr = errMalformedVoting
					return false
				}
			} else {
				parseErr = errMalformedVoting
				return false
			}
		}
		return true
	})
	if parseErr != nil {
		return nil, parseErr
	}
	return &listing, nil
}

var errMalformedVoting = &votingError{}

type votingError struct{}

func (*votingError) Error() string { return "malformed voting list" }

// fetchIsPageLocked reports whether a page is locked (no edit link present).
func (w *WikiDot) fetchIsPageLocked(pageID int64) (bool, error) {
	env, err := w.ajaxJSON(map[string]string{
		"page_id":    strconv.FormatInt(pageID, 10),
		"moduleName": "edit/PageEditModule",
	}, nil, true)
	if err != nil {
		return false, err
	}
	doc, err := htmlx.Parse(env.Message)
	if err != nil {
		return false, err
	}
	return doc.Find("a").Length() == 0, nil
}

// ---- small helpers ----

func attrOr(s *goquery.Selection, attr string) string {
	if v, ok := s.Attr(attr); ok {
		return v
	}
	return ""
}

func parseI64Ptr(s string) *int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func parseIntPrefix(s string) *int64 {
	m := reLeadingInt.FindString(s)
	if m == "" {
		return nil
	}
	n, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

var reLeadingInt = regexp.MustCompile(`^[+-]?[0-9]+`)
