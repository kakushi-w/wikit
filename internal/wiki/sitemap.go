package wiki

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	reMetaDomain   = regexp.MustCompile(`WIKIREQUEST\.info\.domain = "([^"]+)"`)
	reMetaSiteID   = regexp.MustCompile(`WIKIREQUEST\.info\.siteId = (\d+)`)
	reMetaSlug     = regexp.MustCompile(`WIKIREQUEST\.info\.siteUnixName = "([^"]+)"`)
	reMetaHomePage = regexp.MustCompile(`WIKIREQUEST\.info\.pageUnixName = "([^"]+)"`)
	reMetaLang     = regexp.MustCompile(`WIKIREQUEST\.info\.lang = '([^']+)'`)
	rePageXML      = regexp.MustCompile(`_page_([0-9]+)\.xml$`)
)

func (w *WikiDot) fetchSiteMetadata() (SiteMeta, error) {
	body, err := w.get(w.url, nil)
	if err != nil {
		return SiteMeta{}, err
	}
	html := string(body)
	ex := func(re *regexp.Regexp) (string, error) {
		m := re.FindStringSubmatch(html)
		if m == nil {
			return "", fmt.Errorf("regex %v failed on %s for site metadata", re, w.url)
		}
		return m[1], nil
	}
	domain, err := ex(reMetaDomain)
	if err != nil {
		return SiteMeta{}, err
	}
	siteID, err := ex(reMetaSiteID)
	if err != nil {
		return SiteMeta{}, err
	}
	slug, err := ex(reMetaSlug)
	if err != nil {
		return SiteMeta{}, err
	}
	home, err := ex(reMetaHomePage)
	if err != nil {
		return SiteMeta{}, err
	}
	lang, err := ex(reMetaLang)
	if err != nil {
		return SiteMeta{}, err
	}
	return SiteMeta{Domain: domain, SiteID: mustI64(siteID), Slug: slug, HomePage: home, Language: lang}, nil
}

// fetchSiteMap recursively walks sitemap.xml, appending page entries (in document
// order, which the written sitemap.json must preserve).
func (w *WikiDot) fetchSiteMap(sitemapURL string, entries *[]sitemapEntry) error {
	var body []byte
	var lastErr error
	for i := 0; i < 40; i++ {
		w.logf("Fetching %s", sitemapURL)
		b, err := w.get(sitemapURL, nil)
		if err == nil {
			body = b
			break
		}
		w.logf("Exception while fetching %s: %v (tries left: %d)", sitemapURL, err, 40-i-1)
		w.client.ChangeFingerprint()
		time.Sleep(4 * time.Second)
		lastErr = err
	}
	if body == nil {
		return lastErr
	}

	dec := xml.NewDecoder(strings.NewReader(string(body)))
	var inURL, inSitemap bool
	var curLoc, curLastmod string
	var field string

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "url":
				inURL, curLoc, curLastmod = true, "", ""
			case "sitemap":
				inSitemap, curLoc = true, ""
			case "loc":
				field = "loc"
			case "lastmod":
				field = "lastmod"
			}
		case xml.CharData:
			switch field {
			case "loc":
				curLoc += string(t)
			case "lastmod":
				curLastmod += string(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "loc", "lastmod":
				field = ""
			case "sitemap":
				if inSitemap && rePageXML.MatchString(curLoc) {
					if err := w.fetchSiteMap(strings.TrimSpace(curLoc), entries); err != nil {
						return err
					}
				}
				inSitemap = false
			case "url":
				if inURL {
					w.appendSitemapEntry(strings.TrimSpace(curLoc), strings.TrimSpace(curLastmod), entries)
				}
				inURL = false
			}
		}
	}
	return nil
}

func (w *WikiDot) appendSitemapEntry(loc, lastmod string, entries *[]sitemapEntry) {
	if loc == "" {
		return
	}
	if strings.HasPrefix(loc, w.url) {
		loc = loc[len(w.url):]
	} else if strings.HasPrefix(loc, "http") {
		// custom domain: take the path
		if i := strings.Index(loc, "//"); i != -1 {
			rest := loc[i+2:]
			if s := strings.IndexByte(rest, '/'); s != -1 {
				loc = rest[s+1:]
			} else {
				loc = ""
			}
		}
	}
	if loc == "" || loc == "/" {
		return
	}
	if strings.HasPrefix(loc, "/forum/") || strings.HasPrefix(loc, "forum/") {
		return
	}
	loc = strings.TrimPrefix(loc, "/")

	var ms *int64
	if lastmod != "" {
		if t, err := parseLastmod(lastmod); err == nil {
			v := t.UnixMilli()
			ms = &v
		}
	}
	*entries = append(*entries, sitemapEntry{Name: loc, Update: ms})
}

func parseLastmod(s string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"}
	var err error
	for _, l := range layouts {
		var t time.Time
		if t, err = time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, err
}

func findMostRevision(revs []PageRevision) (int64, bool) {
	if len(revs) == 0 {
		return 0, false
	}
	max := revs[0].Revision
	for _, r := range revs {
		if r.Revision > max {
			max = r.Revision
		}
	}
	return max, true
}
