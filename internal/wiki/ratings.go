package wiki

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Sentinels wrapped around each ListPages row so the fullname/rating can be
// recovered from the rendered (tag-stripped) HTML without depending on Wikidot's
// markup. They are plain text, survive strip_tags, and never occur in a
// fullname or a rating value.
const (
	ratingRowSep   = "@@ROW@@"
	ratingFieldSep = "@@F@@"
)

var (
	reListPager  = regexp.MustCompile(`(?i)page\s+(\d+)\s+of\s+(\d+)`)
	reLeadingNum = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?`)
)

// refreshRatings updates page ratings using a bulk ListPages sweep, and for any
// page whose rating changed it also re-fetches the per-user voting list. It only
// rewrites meta/pages/<name>.json — no revisions or files are touched. This
// catches rating/vote changes that leave the sitemap lastmod untouched (voting
// does not create a revision), which the incremental sitemap scan would miss.
func (w *WikiDot) refreshRatings(siteID int64, entries []sitemapEntry) {
	w.logf("Refreshing ratings via ListPages")
	table, err := w.fetchRatingTable(siteID)
	if err != nil {
		w.errf("rating refresh failed: %v", err)
		return
	}
	w.logf("ListPages returned ratings for %d pages", len(table))

	changed := 0
	for _, e := range entries {
		newRating, ok := table[e.Name]
		if !ok {
			continue // page has no rating widget, or not listed
		}
		meta := w.readPageMetadata(e.Name)
		if meta == nil || meta.PageID == 0 {
			continue
		}
		if ratingsEqual(meta.Rating, newRating) {
			continue
		}

		old := "none"
		if meta.Rating != nil {
			old = formatRating(*meta.Rating)
		}
		r := newRating
		meta.Rating = &r

		// The rating moved, so the stored per-user votings are stale; refresh
		// just these pages (one extra request per changed page, not per page).
		if v, verr := w.fetchPageVoters(meta.PageID); verr == nil {
			meta.Votings = v
		} else {
			w.errf("could not refresh voters for %s: %v", e.Name, verr)
		}

		if err := w.writePageMetadata(e.Name, meta); err != nil {
			w.errf("could not write meta for %s: %v", e.Name, err)
			continue
		}
		changed++
		w.logf("rating %s: %s -> %s", e.Name, old, formatRating(newRating))
		w.delay()
	}
	w.logf("Rating refresh done: %d changed of %d pages", changed, len(entries))
}

// fetchRatingTable walks every ListPages result page (Wikidot caps rows per page
// well below the requested perPage, so pagination is mandatory) and returns a
// fullname -> rating map covering the whole wiki.
func (w *WikiDot) fetchRatingTable(siteID int64) (map[string]float64, error) {
	table := map[string]float64{}
	total := 1
	for p := 1; p <= total; p++ {
		w.logf("Fetching ratings page %d/%d", p, total)
		env, err := w.ajaxJSON(map[string]string{
			"moduleName":  "list/ListPagesModule",
			"site":        strconv.FormatInt(siteID, 10),
			"category":    "*",
			"separate":    "false",
			"perPage":     "1000", // ask for the max; Wikidot returns fewer
			"p":           strconv.Itoa(p),
			"module_body": ratingRowSep + "%%fullname%%" + ratingFieldSep + "%%rating%%",
		}, nil, false)
		if err != nil {
			return nil, err
		}

		// Total page count comes from the pager text before we strip it out.
		if m := reListPager.FindStringSubmatch(env.Body); m != nil {
			if n, e := strconv.Atoi(m[2]); e == nil && n > 0 {
				total = n
			}
		}
		parseRatingRows(env.Body, table)
	}
	return table, nil
}

// parseRatingRows extracts fullname/rating pairs from one ListPages HTML body
// into table. The pager block is removed first so its "page X of Y" text can't
// contaminate the last row on the page.
func parseRatingRows(body string, table map[string]float64) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return
	}
	doc.Find("[class*=pager]").Remove()
	text := doc.Text()

	for _, chunk := range strings.Split(text, ratingRowSep) {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		parts := strings.SplitN(chunk, ratingFieldSep, 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		numStr := reLeadingNum.FindString(strings.TrimSpace(parts[1]))
		if name == "" || numStr == "" {
			continue // unrated page (blank %%rating%%) — leave it alone
		}
		f, e := strconv.ParseFloat(numStr, 64)
		if e != nil {
			continue
		}
		table[name] = f
	}
}

func formatRating(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ratingsEqual reports whether the stored rating already equals the new one.
// A missing stored rating counts as different so it gets written.
func ratingsEqual(cur *float64, next float64) bool {
	if cur == nil {
		return false
	}
	return math.Abs(*cur-next) < 1e-9
}
