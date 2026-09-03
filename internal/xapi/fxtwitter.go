package xapi

// Public no-auth fallback via FxTwitter (https://api.fxtwitter.com). It covers a
// single tweet and a user profile without any cookie. Only used when the account
// pool is exhausted or an authed read is rejected, and only when the operator
// opts in, because it leaks the queried id/handle to a third party.

import (
	"encoding/json"
	"fmt"
	"io"

	http "github.com/bogdanfinn/fhttp"
)

const fxAPI = "https://api.fxtwitter.com"

type fxResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Tweet   *fxTweet `json:"tweet"`
	User    *fxUser  `json:"user"`
}

type fxAuthor struct {
	ScreenName string `json:"screen_name"`
	Name       string `json:"name"`
	ID         string `json:"id"`
}

type fxTweet struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Author    fxAuthor `json:"author"`
	Replies   int      `json:"replies"`
	Retweets  int      `json:"retweets"`
	Likes     int      `json:"likes"`
	Quotes    int      `json:"quotes"`
	Views     int      `json:"views"`
	CreatedAt string   `json:"created_at"`
	Lang      string   `json:"lang"`
	Quote     *fxTweet `json:"quote"`
}

type fxUser struct {
	ScreenName  string `json:"screen_name"`
	Name        string `json:"name"`
	ID          string `json:"id"`
	Description string `json:"description"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	Tweets      int    `json:"tweets"`
	Joined      string `json:"joined"`
	Location    string `json:"location"`
}

// FetchTweetPublic fetches one tweet by id from FxTwitter (no auth needed).
func (s *Session) FetchTweetPublic(id string) (*Tweet, error) {
	var r fxResponse
	if err := s.getFxJSON(fxAPI+"/status/"+id, &r); err != nil {
		return nil, err
	}
	if r.Code != 200 || r.Tweet == nil {
		return nil, fmt.Errorf("fxtwitter: tweet %s: code %d %s", id, r.Code, r.Message)
	}
	return fxTweetToModel(r.Tweet), nil
}

// FetchUserPublic fetches a profile by handle from FxTwitter (no auth needed).
func (s *Session) FetchUserPublic(handle string) (*XUser, error) {
	var r fxResponse
	if err := s.getFxJSON(fxAPI+"/"+handle, &r); err != nil {
		return nil, err
	}
	if r.Code != 200 || r.User == nil {
		return nil, fmt.Errorf("fxtwitter: user %s: code %d %s", handle, r.Code, r.Message)
	}
	u := r.User
	return &XUser{
		RestID:         u.ID,
		ScreenName:     u.ScreenName,
		Name:           u.Name,
		Description:    u.Description,
		FollowersCount: u.Followers,
		FriendsCount:   u.Following,
		StatusesCount:  u.Tweets,
		CreatedAt:      u.Joined,
		Location:       u.Location,
		URL:            userURL(u.ScreenName),
	}, nil
}

func fxTweetToModel(t *fxTweet) *Tweet {
	return &Tweet{
		RestID:         t.ID,
		UserScreenName: t.Author.ScreenName,
		UserName:       t.Author.Name,
		CreatedAt:      t.CreatedAt,
		Text:           t.Text,
		Lang:           t.Lang,
		ReplyCount:     t.Replies,
		RetweetCount:   t.Retweets,
		LikeCount:      t.Likes,
		QuoteCount:     t.Quotes,
		ViewCount:      t.Views,
		IsQuote:        t.Quote != nil,
		URL:            tweetURL(t.Author.ScreenName, t.ID),
	}
}

// getFxJSON performs a browser-like GET (no cookie) and decodes JSON.
func (s *Session) getFxJSON(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header = http.Header{
		"accept":            {"application/json"},
		"user-agent":        {s.userAgent},
		http.HeaderOrderKey: {"accept", "user-agent"},
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fxtwitter: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fxtwitter: http %d", resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}
