package xapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseArticles verifies parseArticles maps a real ArticleEntitiesSlice item
// (a captured published Article) into an Article, flattening the content blocks.
func TestParseArticles(t *testing.T) {
	const raw = `{"data":{"user":{"result":{"__typename":"User","articles_article_mixer_slice":{"items":[{"article_entity_results":{"result":{"content_state":{"blocks":[{"key":"1edft","text":"First paragraph of the article.","type":"unstyled"},{"key":"b8m9m","text":"Second paragraph of the article.","type":"unstyled"}],"entityMap":[]},"cover_media":{"media_id":"3001","media_info":{"__typename":"ApiImage","original_img_url":"https://pbs.twimg.com/media/EXAMPLE001.jpg"},"media_key":"3_3001"},"lifecycle_state":{"lifecycle":"Published","modified_at_secs":1770281982},"metadata":{"author_results":{"result":{"__typename":"User","core":{"screen_name":"alice"},"rest_id":"111"}},"created_at_secs":1770280855,"first_published_at_secs":1770281982,"modified_at_secs":1770281919,"tweet_results":{"result":{"__typename":"Tweet","rest_id":"2001"}}},"preview_text":"First paragraph of the article.","rest_id":"1001","title":"Example Article Title"}}}],"slice_info":{"next_cursor":"DAACCgABAAAAAAAAAAAIAAIAAAABAAA"}}}}}}`

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	arts, cursor := parseArticles(m)
	if len(arts) != 1 {
		t.Fatalf("parseArticles returned %d articles, want 1", len(arts))
	}
	a := arts[0]
	if a.RestID != "1001" {
		t.Errorf("RestID = %q", a.RestID)
	}
	if a.Title != "Example Article Title" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Lifecycle != "Published" {
		t.Errorf("Lifecycle = %q, want Published", a.Lifecycle)
	}
	if a.AuthorID != "111" || a.AuthorScreenName != "alice" {
		t.Errorf("author = %q/%q", a.AuthorID, a.AuthorScreenName)
	}
	if a.TweetID != "2001" {
		t.Errorf("TweetID = %q", a.TweetID)
	}
	if a.CoverImageURL != "https://pbs.twimg.com/media/EXAMPLE001.jpg" {
		t.Errorf("CoverImageURL = %q", a.CoverImageURL)
	}
	if a.FirstPublishedAt != 1770281982 {
		t.Errorf("FirstPublishedAt = %d", a.FirstPublishedAt)
	}
	if !strings.HasPrefix(a.Text, "İngiltere'de şirket kurup") || !strings.Contains(a.Text, "\nKendi tecrübelerim") {
		t.Errorf("Text not flattened correctly: %q", a.Text)
	}
	if cursor != "DAACCgABAAAAAAAAAAAIAAIAAAABAAA" {
		t.Errorf("cursor = %q", cursor)
	}
}
