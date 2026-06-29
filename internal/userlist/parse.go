package userlist

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var (
	reRealName   = regexp.MustCompile(`(?i)Real name`)
	reGender     = regexp.MustCompile(`(?i)Gender`)
	reBirthday   = regexp.MustCompile(`(?i)Birthday`)
	reFrom       = regexp.MustCompile(`(?i)From`)
	reWebsite    = regexp.MustCompile(`(?i)Website`)
	reUserSince  = regexp.MustCompile(`(?i)Wikidot User since:`)
	reBio        = regexp.MustCompile(`(?i)About`)
	reAcctType   = regexp.MustCompile(`(?i)Account type`)
	reActivity   = regexp.MustCompile(`(?i)Karma level`)
	reUserIDClick = regexp.MustCompile(`(?i)^WIKIDOT\.modules\.UserInfoModule\.listeners\.(?:addContact|flagUser)\(event,([0-9]+)\)$`)
)

var activityLevels = []struct {
	re  *regexp.Regexp
	val UserActivity
}{
	{regexp.MustCompile(`(?i)very high`), ActivityVeryHigh}, // check before "high"
	{regexp.MustCompile(`(?i)none`), ActivityNone},
	{regexp.MustCompile(`(?i)low`), ActivityLow},
	{regexp.MustCompile(`(?i)medium`), ActivityMedium},
	{regexp.MustCompile(`(?i)high`), ActivityHigh},
	{regexp.MustCompile(`(?i)guru`), ActivityGuru},
}

// parseUserInfo mirrors WikiDotUserList.parseBody. It returns (user, notFound,
// err). notFound is true when the profile explicitly says the user is gone.
func parseUserInfo(doc *goquery.Document, username string) (User, bool, error) {
	div1 := doc.Find("div.col-md-9").First()
	if div1.Length() == 0 {
		eb := doc.Find("div.error-block").First()
		if eb.Length() > 0 && strings.EqualFold(strings.TrimSpace(eb.Text()), "user does not exist.") {
			return User{}, true, fmt.Errorf("user %s does not exist", username)
		}
		return User{}, false, fmt.Errorf("div.col-md-9 is missing")
	}

	title := div1.Find("h1.profile-title").First()
	info := div1.Find("dl.dl-horizontal").First()
	if title.Length() == 0 {
		return User{}, false, fmt.Errorf("profile-title is missing")
	}
	if info.Length() == 0 {
		return User{}, false, fmt.Errorf("dl.dl-horizontal is missing")
	}

	dts := info.Find("dt")
	dds := info.Find("dd")
	if dts.Length() != dds.Length() {
		return User{}, false, fmt.Errorf("dt and dd length mismatch")
	}

	matched := map[string]string{}
	matchers := []struct {
		key string
		re  *regexp.Regexp
	}{
		{"real_name", reRealName}, {"gender", reGender}, {"birthday", reBirthday},
		{"from", reFrom}, {"website", reWebsite}, {"wikidot_user_since", reUserSince},
		{"bio", reBio}, {"account_type", reAcctType}, {"activity", reActivity},
	}
	dts.Each(func(i int, dt *goquery.Selection) {
		key := dt.Text()
		val := strings.TrimSpace(dds.Eq(i).Text())
		for _, m := range matchers {
			if m.re.MatchString(key) {
				matched[m.key] = val
			}
		}
	})

	if _, ok := matched["wikidot_user_since"]; !ok {
		return User{}, false, fmt.Errorf("wikidot_user_since is missing")
	}

	var userID int64
	found := false
	div1.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if oc, ok := a.Attr("onclick"); ok {
			if m := reUserIDClick.FindStringSubmatch(oc); m != nil {
				userID = mustI64(m[1])
				found = true
				return false
			}
		}
		return true
	})
	if !found {
		return User{}, false, fmt.Errorf("can't determine user id for %s", username)
	}

	u := User{
		FullName:         strings.TrimSpace(title.Text()),
		Username:         username,
		WikidotUserSince: jsDateMillis(matched["wikidot_user_since"]) / 1000,
		FetchedAt:        nowMillis(),
		UserID:           userID,
	}
	u.RealName = optFromMap(matched, "real_name")
	u.Gender = optFromMap(matched, "gender")
	u.From = optFromMap(matched, "from")
	u.Website = optFromMap(matched, "website")
	u.Bio = optFromMap(matched, "bio")
	u.AccountType = optFromMap(matched, "account_type")
	if b, ok := matched["birthday"]; ok {
		ms := jsDateMillis(b)
		u.Birthday = &ms
	}
	activity := int64(ActivityUnknown)
	if a, ok := matched["activity"]; ok {
		for _, lvl := range activityLevels {
			if lvl.re.MatchString(a) {
				activity = int64(lvl.val)
				break
			}
		}
	}
	u.Activity = &activity

	return u, false, nil
}

func optFromMap(m map[string]string, key string) *string {
	if v, ok := m[key]; ok {
		return &v
	}
	return nil
}

func mustI64(s string) int64 {
	var n int64
	for i := 0; i < len(s); i++ {
		n = n*10 + int64(s[i]-'0')
	}
	return n
}

// jsDateMillis parses a date string roughly the way JavaScript's Date does for
// the formats Wikidot emits, returning epoch milliseconds (0 on failure).
func jsDateMillis(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	layouts := []string{
		"02 Jan 2006 15:04",
		"02 Jan 2006, 15:04",
		"2 Jan 2006 15:04",
		"02 Jan 2006",
		"2 Jan 2006",
		"Jan 2, 2006",
		"January 2, 2006",
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}
