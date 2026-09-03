package xapi

import "strings"

// UserArticles returns a user's long-form Articles for a lifecycle ("Published"
// or "Draft"). Draft articles are only visible to their own account, so a Draft
// read must use that account. An empty lifecycle defaults to "Published".
func (c *XClient) UserArticles(handle, lifecycle string, count int, cursor string) ([]Article, string, error) {
	uid, err := c.resolveUID(handle)
	if err != nil {
		return nil, "", err
	}
	if lifecycle == "" {
		lifecycle = "Published"
	}
	return paginate(c, "ArticleEntitiesSlice",
		map[string]any{"userId": uid, "lifecycle": lifecycle},
		parseArticles, articleKey, count, cursor)
}

func articleKey(a Article) string { return a.RestID }

// parseEmbeddedArticle maps an Article carried inline by a tweet node
// (article.article_results.result), or returns nil when the tweet has none. The
// embedded node reuses articleFromResult; its author/tweet metadata comes from
// the parent tweet, so those Article fields stay empty here.
func parseEmbeddedArticle(t map[string]any) *Article {
	r := asMap(dig(t, "article", "article_results", "result"))
	if r == nil {
		return nil
	}
	return articleFromResult(r)
}

// parseArticles maps the articles_article_mixer_slice items to Article records and
// returns the slice's bottom cursor.
func parseArticles(payload map[string]any) ([]Article, string) {
	slice := asMap(dig(payload, "data", "user", "result", "articles_article_mixer_slice"))
	var out []Article
	for _, it := range asSlice(slice["items"]) {
		r := asMap(dig(asMap(it), "article_entity_results", "result"))
		if a := articleFromResult(r); a != nil {
			out = append(out, *a)
		}
	}
	return out, asString(dig(slice, "slice_info", "next_cursor"))
}

// articleFromResult maps one article_entity_results.result node to an Article,
// returning nil when it carries no rest_id.
func articleFromResult(r map[string]any) *Article {
	id := asString(r["rest_id"])
	if id == "" {
		return nil
	}
	return &Article{
		RestID:           id,
		Title:            asString(r["title"]),
		PreviewText:      asString(r["preview_text"]),
		Text:             flattenArticleBlocks(r),
		CoverImageURL:    asString(dig(r, "cover_media", "media_info", "original_img_url")),
		Lifecycle:        asString(dig(r, "lifecycle_state", "lifecycle")),
		CreatedAt:        asInt64(dig(r, "metadata", "created_at_secs")),
		FirstPublishedAt: asInt64(dig(r, "metadata", "first_published_at_secs")),
		ModifiedAt:       asInt64(dig(r, "metadata", "modified_at_secs")),
		TweetID:          asString(dig(r, "metadata", "tweet_results", "result", "rest_id")),
		AuthorID:         asString(dig(r, "metadata", "author_results", "result", "rest_id")),
		AuthorScreenName: asString(dig(r, "metadata", "author_results", "result", "core", "screen_name")),
	}
}

// flattenArticleBlocks joins the article's content blocks into plain text, one
// block per line.
func flattenArticleBlocks(r map[string]any) string {
	var sb strings.Builder
	for i, b := range asSlice(dig(r, "content_state", "blocks")) {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(asString(asMap(b)["text"]))
	}
	return sb.String()
}
