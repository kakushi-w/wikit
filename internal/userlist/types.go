package userlist

import (
	"strconv"

	"wikit/internal/jsonx"
)

// UserActivity mirrors the original enum; it is serialized as its numeric value.
type UserActivity int

const (
	ActivityNone UserActivity = iota
	ActivityLow
	ActivityMedium
	ActivityHigh
	ActivityVeryHigh
	ActivityGuru
	ActivityUnknown
)

// User mirrors the original User interface. Optional fields are pointers so they
// are omitted (not null) when absent, matching JSON.stringify.
type User struct {
	FullName string
	Username string

	RealName *string
	Gender   *string
	Birthday *int64
	From     *string
	Website  *string

	WikidotUserSince int64
	Bio              *string

	AccountType *string
	Activity    *int64

	FetchedAt int64
	UserID    int64
}

func (u User) object() *jsonx.Object {
	o := jsonx.NewObject()
	o.Set("full_name", u.FullName)
	o.Set("username", u.Username)
	if u.RealName != nil {
		o.Set("real_name", *u.RealName)
	}
	if u.Gender != nil {
		o.Set("gender", *u.Gender)
	}
	if u.Birthday != nil {
		o.Set("birthday", *u.Birthday)
	}
	if u.From != nil {
		o.Set("from", *u.From)
	}
	if u.Website != nil {
		o.Set("website", *u.Website)
	}
	o.Set("wikidot_user_since", u.WikidotUserSince)
	if u.Bio != nil {
		o.Set("bio", *u.Bio)
	}
	if u.AccountType != nil {
		o.Set("account_type", *u.AccountType)
	}
	if u.Activity != nil {
		o.Set("activity", *u.Activity)
	}
	o.Set("fetched_at", u.FetchedAt)
	o.Set("user_id", u.UserID)
	return o
}

func readUser(o *jsonx.Object) User {
	u := User{
		FullName:         getStr(o, "full_name"),
		Username:         getStr(o, "username"),
		RealName:         optStr(o, "real_name"),
		Gender:           optStr(o, "gender"),
		Birthday:         optInt(o, "birthday"),
		From:             optStr(o, "from"),
		Website:          optStr(o, "website"),
		WikidotUserSince: getInt(o, "wikidot_user_since"),
		Bio:              optStr(o, "bio"),
		AccountType:      optStr(o, "account_type"),
		Activity:         optInt(o, "activity"),
		FetchedAt:        getInt(o, "fetched_at"),
		UserID:           getInt(o, "user_id"),
	}
	return u
}

// bucketOf returns the bucket id for a user id (id >> 13), matching the original.
func bucketOf(id int64) int64 { return id >> 13 }

func getStr(o *jsonx.Object, key string) string {
	if v, ok := o.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(o *jsonx.Object, key string) int64 {
	if v, ok := o.Get(key); ok {
		return toInt(v)
	}
	return 0
}

func optStr(o *jsonx.Object, key string) *string {
	if v, ok := o.Get(key); ok && v != nil {
		if s, ok := v.(string); ok {
			return &s
		}
	}
	return nil
}

func optInt(o *jsonx.Object, key string) *int64 {
	if v, ok := o.Get(key); ok && v != nil {
		i := toInt(v)
		return &i
	}
	return nil
}

func toInt(v any) int64 {
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
