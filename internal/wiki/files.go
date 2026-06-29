package wiki

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"wikit/internal/htmlx"
	"wikit/internal/httpc"
)

var (
	reFileRow  = regexp.MustCompile(`(?i)file-row-([0-9]+)`)
	reFileSize = regexp.MustCompile(`(?i)([0-9]+) bytes`)
	reTiff     = regexp.MustCompile(`(?i)\[TIFF image data.+?\]`)
)

// fetchFileList returns the file ids listed on one page of a page's files.
func (w *WikiDot) fetchFileList(pageID int64, page int) ([]int64, error) {
	env, err := w.ajaxJSON(map[string]string{
		"page_id":    strconv.FormatInt(pageID, 10),
		"page":       strconv.Itoa(page),
		"moduleName": "files/PageFilesModule",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	doc, err := htmlx.Parse(env.Body)
	if err != nil {
		return nil, err
	}
	var ids []int64
	doc.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		if id, ok := tr.Attr("id"); ok {
			if m := reFileRow.FindStringSubmatch(id); m != nil {
				ids = append(ids, mustI64(m[1]))
			}
		}
	})
	return ids, nil
}

func (w *WikiDot) fetchFileListForce(pageID int64) []int64 {
	var listing []int64
	for page := 0; ; page++ {
		for {
			got, err := w.fetchFileList(pageID, page)
			if err != nil {
				w.errf("Encountered %v when fetching file list, sleeping for 2 seconds", err)
				time.Sleep(2 * time.Second)
				continue
			}
			listing = append(listing, got...)
			if len(got) < 100 {
				return listing
			}
			break
		}
	}
}

// fetchFileMeta fetches metadata for a single file; returns nil when the file no
// longer exists.
func (w *WikiDot) fetchFileMeta(fileID int64) (*FileMeta, error) {
	w.logf("Fetching file meta of %d", fileID)
	env, err := w.ajaxJSON(map[string]string{
		"file_id":    strconv.FormatInt(fileID, 10),
		"moduleName": "files/FileInformationWinModule",
	}, nil, true)
	if err != nil {
		return nil, err
	}
	if env.Status == "wrong_file" {
		return nil, nil
	}
	body := reTiff.ReplaceAllString(env.Body, "")
	doc, err := htmlx.Parse(body)
	if err != nil {
		return nil, err
	}
	rows := doc.Find("tr")
	cell := func(rowIdx int) *goquery.Selection {
		return rows.Eq(rowIdx).Find("td").Eq(1)
	}

	name := cell(0)
	fullURL := cell(1)
	size := cell(2)
	mime := cell(3)
	contentType := cell(4)
	uploader := cell(5)
	date := cell(6)

	href, _ := fullURL.Find("a").First().Attr("href")
	sizeText := strings.TrimSpace(size.Text())
	var sizeBytes int64
	if m := reFileSize.FindStringSubmatch(sizeText); m != nil {
		sizeBytes = mustI64(m[1])
	}
	var stamp int64
	if cls, ok := date.Find("span.odate").First().Attr("class"); ok {
		if m := reDateClass.FindStringSubmatch(cls); m != nil {
			stamp = mustI64(m[1])
		}
	}
	iv := int64(fileMetadataVersion)
	return &FileMeta{
		FileID:          fileID,
		Name:            strings.TrimSpace(name.Text()),
		URL:             href,
		Size:            sizeText,
		SizeBytes:       sizeBytes,
		Mime:            strings.TrimSpace(mime.Text()),
		Content:         strings.TrimSpace(contentType.Text()),
		Author:          w.matchAndFetchUser(uploader),
		Stamp:           stamp,
		InternalVersion: &iv,
	}, nil
}

func (w *WikiDot) fetchFileMetaForce(fileID int64) *FileMeta {
	for {
		m, err := w.fetchFileMeta(fileID)
		if err == nil {
			return m
		}
		w.errf("Encountered %v when fetching file meta (ID %d), sleeping for 2 seconds", err, fileID)
		time.Sleep(2 * time.Second)
	}
}

// fetchFileMetaListForce returns metadata for all files of a page, reusing
// existing entries whose internal_version is current.
func (w *WikiDot) fetchFileMetaListForce(pageID int64, existing []FileMeta) []FileMeta {
	var out []FileMeta
	for _, fid := range w.fetchFileListForce(pageID) {
		hit := false
		for _, e := range existing {
			if e.FileID == fid && e.InternalVersion != nil && *e.InternalVersion >= fileMetadataVersion {
				out = append(out, e)
				hit = true
				break
			}
		}
		if !hit {
			if m := w.fetchFileMetaForce(fid); m != nil {
				out = append(out, *m)
			}
		}
	}
	return out
}

// fetchFilesFor fetches metadata for and downloads all files of a page.
func (w *WikiDot) fetchFilesFor(pageID int64, existing []FileMeta) []FileMeta {
	var metadata []FileMeta
	for _, fm := range w.fetchFileMetaListForce(pageID, existing) {
		pageName, fileName, _, ok := splitFilePathRaw(fm.URL)
		if !ok {
			continue
		}
		metadata = append(metadata, fm)
		w.state.setFileMap(fm.FileID, fileMapEntry{URL: fm.URL, Path: pageName + "/" + fileName})

		if w.fileExists(pageName, fm.FileID, fm.SizeBytes) {
			continue
		}
		w.fetchFileInner(fm.URL, fm.FileID, pageName)
	}
	return metadata
}

func (w *WikiDot) fileExists(pageName string, fileID int64, size int64) bool {
	st, err := os.Stat(filepath.Join(w.workDir, "files", pageName, strconv.FormatInt(fileID, 10)))
	if err != nil {
		return false
	}
	if size != 0 && st.Size() != size {
		return false
	}
	return true
}

func (w *WikiDot) fetchFileInner(fileURL string, fileID int64, pageName string) {
	w.logf("Fetching file %s", fileURL)
	w.state.pushPendingFile(fileID)
	data, err := w.get(fileURL, nil)
	if err != nil {
		w.logf("Unable to fetch %s because %v", fileURL, err)
		if he, ok := err.(*httpc.HTTPError); ok && he.Status == 404 {
			w.state.removePendingFile(fileID)
		}
		return
	}
	dir := filepath.Join(w.workDir, "files", pageName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.errf("mkdir %s: %v", dir, err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.FormatInt(fileID, 10)), data, 0o644); err != nil {
		w.errf("write file %d: %v", fileID, err)
		return
	}
	w.state.removePendingFile(fileID)
}
