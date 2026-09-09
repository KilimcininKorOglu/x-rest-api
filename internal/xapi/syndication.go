package xapi

// Tier 0 of the credential-free read path: x.com's own public embed/syndication
// surface. It needs no cookie and no guest token, so it is the cheapest fallback
// and, unlike the FxTwitter reader, it keeps the query on x.com's own hosts.
// It serves a single tweet only; timelines and search are not exposed here.

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

const synTweetAPI = "https://cdn.syndication.twimg.com/tweet-result"

// base36Digits indexes a base-36 digit by its value.
const base36Digits = "0123456789abcdefghijklmnopqrstuvwxyz"

// synFracDigits is how many base-36 fraction digits the token carries. The
// endpoint only checks that the token matches the id, so a fixed width is enough.
const synFracDigits = 20

// syndicationToken derives the `token` query parameter the syndication endpoint
// requires for a tweet id. The value is the id scaled into a small float, written
// in base 36, with the zeros and the radix point removed.
func syndicationToken(id string) string {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return ""
	}
	x := (float64(n) / 1e15) * math.Pi

	whole := uint64(x)
	frac := x - float64(whole)

	var b strings.Builder
	b.WriteString(base36Uint(whole))
	b.WriteByte('.')
	for range synFracDigits {
		frac *= 36
		d := int(frac)
		b.WriteByte(base36Digits[d])
		frac -= float64(d)
	}
	r := strings.NewReplacer("0", "", ".", "")
	return r.Replace(b.String())
}

// base36Uint writes n in base 36, most significant digit first.
func base36Uint(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = base36Digits[n%36]
		n /= 36
	}
	return string(buf[i:])
}

// synTweet is the subset of the syndication tweet payload this API maps. The
// endpoint uses its own field names, not the GraphQL ones.
type synTweet struct {
	IDStr             string `json:"id_str"`
	Text              string `json:"text"`
	CreatedAt         string `json:"created_at"`
	Lang              string `json:"lang"`
	FavoriteCount     int    `json:"favorite_count"`
	ConversationCount int    `json:"conversation_count"`
	User              struct {
		IDStr      string `json:"id_str"`
		ScreenName string `json:"screen_name"`
		Name       string `json:"name"`
	} `json:"user"`
}

// FetchTweetSyndication fetches one tweet from the public syndication endpoint.
// It needs no credential of any kind.
func (s *Session) FetchTweetSyndication(id string) (*Tweet, error) {
	token := syndicationToken(id)
	if token == "" {
		return nil, fmt.Errorf("syndication: %q is not a numeric tweet id", id)
	}
	q := url.Values{"id": {id}, "token": {token}, "lang": {"en"}}
	var t synTweet
	if err := s.getPublicJSON("syndication", synTweetAPI+"?"+q.Encode(), &t); err != nil {
		return nil, err
	}
	if t.IDStr == "" {
		return nil, fmt.Errorf("syndication: tweet %s: no result", id)
	}
	return synTweetToModel(t), nil
}

// synTweetToModel maps a syndication payload onto the shared Tweet model. The
// syndication surface carries no repost or view counters, so those stay zero.
func synTweetToModel(t synTweet) *Tweet {
	return &Tweet{
		RestID:         t.IDStr,
		UserScreenName: t.User.ScreenName,
		UserName:       t.User.Name,
		CreatedAt:      t.CreatedAt,
		Text:           t.Text,
		Lang:           t.Lang,
		LikeCount:      t.FavoriteCount,
		ReplyCount:     t.ConversationCount,
		URL:            tweetURL(t.User.ScreenName, t.IDStr),
	}
}
