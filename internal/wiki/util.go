package wiki

import (
	"net/url"
	"strconv"
	"strings"
)

// mustI64 parses a string of digits (already validated by a regex) to int64.
func mustI64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// decodeURIComponent decodes %XX escapes like JavaScript's decodeURIComponent
// (notably leaving '+' untouched, unlike query unescaping).
func decodeURIComponent(s string) string {
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	return s
}

// reencodingTable lists the characters unsafe on disk (Windows NT kernel limits)
// and their percent-encodings, matching the original reencoding_table.
var reencodingReplacer = strings.NewReplacer(
	`\`, "%5C",
	`:`, "%3A",
	`*`, "%2A",
	`?`, "%3F",
	`"`, "%22",
	`<`, "%3C",
	`>`, "%3E",
	`|`, "%7C",
	`/`, "%2F",
)

// reencodeComponent decodes then re-encodes a path component so it is safe to
// store on disk, exactly as the original does for file names.
func reencodeComponent(s string) string {
	return reencodingReplacer.Replace(decodeURIComponent(s))
}

// splitFilePath splits a "<page>/<file>" path into reencoded components and the
// recombined "<page>/<file>" path, mirroring WikiDot.splitFilePath.
func splitFilePath(path string) (pageName, fileName, recombined string) {
	first := strings.IndexByte(path, '/')
	if first == -1 {
		pageName = "~"
		fileName = reencodeComponent(path)
	} else {
		pageName = path[:first]
		if pageName != "" {
			pageName = reencodeComponent(pageName)
		} else {
			pageName = "~"
		}
		fileName = reencodeComponent(path[first+1:])
	}
	if fileName == "." || fileName == ".." {
		fileName = "why_did_you_name_me_this_way"
	}
	if pageName == "." || pageName == ".." {
		pageName = "why_did_you_name_me_this_way"
	}
	return pageName, fileName, pageName + "/" + fileName
}

// splitFilePathRaw extracts the local--files portion of a URL and splits it.
func splitFilePathRaw(fileURL string) (pageName, fileName, recombined string, ok bool) {
	idx := strings.Index(fileURL, "/local--files/")
	if idx == -1 {
		return "", "", "", false
	}
	rest := fileURL[idx+len("/local--files/"):]
	p, f, r := splitFilePath(rest)
	return p, f, r, true
}
