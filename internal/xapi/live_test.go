//go:build live

package xapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// TestLiveBookmark verifies the transaction-id fix against the real API. Run with:
//
//	go test -tags live -run TestLiveBookmark -count=1 ./internal/xapi/
//
// It needs a cookie.txt (browser cookie export JSON) at the repo root.
func TestLiveBookmark(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	if err := c.CreateBookmark("20"); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}
	t.Log("CreateBookmark: OK")
	if err := c.DeleteBookmark("20"); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
	t.Log("DeleteBookmark: OK (cleaned up)")
}

// TestLiveMedia uploads a small PNG, posts a tweet that attaches it, then deletes
// the tweet. Run with:
//
//	go test -tags live -run TestLiveMedia -count=1 -v ./internal/xapi/
func TestLiveMedia(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	mediaID, err := c.UploadMedia(makePNG(), "image/png")
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	t.Logf("UploadMedia: media_id=%s", mediaID)

	tw, err := c.CreateTweet("", "", []string{mediaID}, "")
	if err != nil {
		t.Fatalf("CreateTweet(media): %v", err)
	}
	t.Logf("CreateTweet(media): rest_id=%s", tw.RestID)

	if err := c.DeleteTweet(tw.RestID); err != nil {
		t.Fatalf("DeleteTweet: %v", err)
	}
	t.Log("DeleteTweet: OK (cleaned up)")
}

// TestLiveRetweet reposts a permanent tweet, then removes the repost, verifying
// the CreateRetweet queryId. Run with:
//
//	go test -tags live -run TestLiveRetweet -count=1 -v ./internal/xapi/
func TestLiveRetweet(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	id, err := c.CreateRetweet("20")
	if err != nil {
		t.Fatalf("CreateRetweet: %v", err)
	}
	t.Logf("CreateRetweet: rest_id=%s", id)
	if err := c.DeleteRetweet("20"); err != nil {
		t.Fatalf("DeleteRetweet: %v", err)
	}
	t.Log("DeleteRetweet: OK (cleaned up)")
}

// TestLiveQueryIDRefresh checks that the bundle refresh reaches x.com's client
// bundle and extracts the live write-mutation queryIds. Run with:
//
//	go test -tags live -run TestLiveQueryIDRefresh -count=1 -v ./internal/xapi/
func TestLiveQueryIDRefresh(t *testing.T) {
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ids, err := sess.FetchQueryIDs()
	if err != nil {
		t.Fatalf("FetchQueryIDs: %v", err)
	}
	t.Logf("fetched %d operationName->queryId pairs", len(ids))
	for _, op := range []string{"CreateTweet", "CreateRetweet", "CreateBookmark", "FavoriteTweet", "DeleteTweet"} {
		if q := ids[op]; q != "" {
			t.Logf("  %-16s %s", op, q)
		} else {
			t.Logf("  %-16s MISSING", op)
		}
	}
}

// makePNG returns a small solid-color PNG for the media upload test.
func makePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 0x1d, G: 0x9b, B: 0xf0, A: 0xff})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// loadLiveAccount reads auth_token and ct0 from the repo-root cookie.txt export.
func loadLiveAccount(t *testing.T) Account {
	t.Helper()
	raw, err := os.ReadFile("../../cookie.txt")
	if err != nil {
		t.Skipf("no cookie.txt: %v", err)
	}
	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &cookies); err != nil {
		t.Fatalf("parse cookie.txt: %v", err)
	}
	var a Account
	a.ID = 1
	for _, ck := range cookies {
		switch ck.Name {
		case "auth_token":
			a.AuthToken = ck.Value
		case "ct0":
			a.CT0 = ck.Value
		}
	}
	if a.AuthToken == "" || a.CT0 == "" {
		t.Fatal("cookie.txt missing auth_token or ct0")
	}
	return a
}
