package xapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

// Media upload over the v1.1 endpoint via the chunked INIT/APPEND/FINALIZE flow
// (plus STATUS polling for video). INIT carries media_type so x.com recognizes the
// content. Not GraphQL, so it reuses the session auth headers on a hand-built
// request.

const mediaUploadURL = "https://upload.x.com/i/media/upload.json"

// mediaChunkSize is the per-APPEND chunk size (x.com caps a chunk at 5 MB).
const mediaChunkSize = 4 << 20

// UploadMedia uploads media bytes and returns the media_id_string to attach to a
// tweet. It runs INIT, chunked APPEND, FINALIZE, then waits for processing when
// the media needs it (video/gif).
func (c *XClient) UploadMedia(data []byte, mediaType string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("UploadMedia: empty media")
	}
	mediaID, err := c.mediaInit(len(data), mediaType)
	if err != nil {
		return "", err
	}
	if err := c.mediaAppend(mediaID, data); err != nil {
		return "", err
	}
	info, err := c.mediaFinalize(mediaID)
	if err != nil {
		return "", err
	}
	if mediaNeedsProcessing(info) {
		if err := c.mediaAwait(mediaID); err != nil {
			return "", err
		}
	}
	return mediaID, nil
}

// mediaIDFrom pulls the media_id_string out of an upload response.
func mediaIDFrom(out map[string]any) (string, error) {
	id := asString(out["media_id_string"])
	if id == "" {
		return "", fmt.Errorf("media upload: no media_id_string in response")
	}
	return id, nil
}

// mediaInit starts a chunked upload and returns the media_id_string.
func (c *XClient) mediaInit(size int, mediaType string) (string, error) {
	form := url.Values{
		"command":     {"INIT"},
		"media_type":  {mediaType},
		"total_bytes": {strconv.Itoa(size)},
	}
	if cat := mediaCategory(mediaType); cat != "" {
		form.Set("media_category", cat)
	}
	out, err := c.callForm("MediaInit", mediaUploadURL, form)
	if err != nil {
		return "", err
	}
	return mediaIDFrom(out)
}

// mediaCategory maps a MIME type to x.com's media_category, which INIT needs so
// the media can be attached to a tweet.
func mediaCategory(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/gif"):
		return "tweet_gif"
	case strings.HasPrefix(mimeType, "image/"):
		return "tweet_image"
	case strings.HasPrefix(mimeType, "video/"):
		return "tweet_video"
	}
	return ""
}

// mediaAppend uploads the media in chunks as multipart requests.
func (c *XClient) mediaAppend(mediaID string, data []byte) error {
	for i, off := 0, 0; off < len(data); i, off = i+1, off+mediaChunkSize {
		end := min(off+mediaChunkSize, len(data))
		fields := map[string]string{
			"command": "APPEND", "media_id": mediaID, "segment_index": strconv.Itoa(i),
		}
		if _, err := c.mediaMultipart("MediaAppend", fields, data[off:end]); err != nil {
			return err
		}
	}
	return nil
}

// mediaFinalize completes a chunked upload; the response may carry processing_info.
func (c *XClient) mediaFinalize(mediaID string) (map[string]any, error) {
	return c.callForm("MediaFinalize", mediaUploadURL, url.Values{
		"command":  {"FINALIZE"},
		"media_id": {mediaID},
	})
}

// mediaMultipart posts a multipart/form-data request with extra text fields plus
// the media bytes as the "media" file field, and decodes the JSON response.
func (c *XClient) mediaMultipart(op string, fields map[string]string, media []byte) (map[string]any, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, err := mw.CreateFormFile("media", "blob")
	if err != nil {
		return nil, fmt.Errorf("%s: form file: %w", op, err)
	}
	if _, err := fw.Write(media); err != nil {
		return nil, fmt.Errorf("%s: write media: %w", op, err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("%s: close: %w", op, err)
	}
	req, err := http.NewRequest(http.MethodPost, mediaUploadURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	req.Header = c.sess.headers(c.acct, "en", "")
	// Override in place with the lowercase key headers() uses; Header.Set would add
	// a second canonical "Content-Type", and x.com then reads the json one.
	req.Header["content-type"] = []string{mw.FormDataContentType()}
	return c.doUpload(op, req)
}

// mediaNeedsProcessing reports whether FINALIZE returned a pending processing_info.
func mediaNeedsProcessing(info map[string]any) bool {
	pi := asMap(info["processing_info"])
	if pi == nil {
		return false
	}
	state := asString(pi["state"])
	return state == "pending" || state == "in_progress"
}

// mediaAwait polls STATUS until the media finishes processing or fails.
func (c *XClient) mediaAwait(mediaID string) error {
	for range 20 {
		time.Sleep(3 * time.Second)
		u := mediaUploadURL + "?command=STATUS&media_id=" + url.QueryEscape(mediaID)
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return fmt.Errorf("MediaStatus: %w", err)
		}
		req.Header = c.sess.headers(c.acct, "en", "")
		out, err := c.doUpload("MediaStatus", req)
		if err != nil {
			return err
		}
		switch asString(dig(out, "processing_info", "state")) {
		case "succeeded":
			return nil
		case "failed":
			return fmt.Errorf("MediaStatus: media processing failed")
		}
	}
	return fmt.Errorf("MediaStatus: media processing timed out")
}

// doUpload sends an upload request and decodes the JSON response.
func (c *XClient) doUpload(op string, req *http.Request) (map[string]any, error) {
	resp, err := c.sess.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()
	c.rateLimit = parseRateLimit(resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read body: %w", op, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, msg := parseXErrors(body)
		return nil, &UpstreamError{
			Op: op, Status: resp.StatusCode, Body: truncate(body, 300),
			Code: code, Msg: msg, HTML: isHTMLBlock(resp.Header, body),
		}
	}
	var out map[string]any
	if len(body) > 0 {
		_ = json.Unmarshal(body, &out)
	}
	return out, nil
}
