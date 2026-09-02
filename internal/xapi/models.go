package xapi

import "fmt"

// XUser is a flat profile record. Callers (a monitor, a CSV export, a DB row)
// UserIdentity is the minimal id<->username mapping for a user, without the rest
// of the profile.
type UserIdentity struct {
	ID       string `json:"id"`       // numeric user id (rest_id)
	Username string `json:"username"` // screen_name
}

// do not have to know about x.com's nested GraphQL shapes.
type XUser struct {
	RestID           string     `json:"rest_id"` // numeric user id (stable key; screen_names change)
	ScreenName       string     `json:"screen_name"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	FollowersCount   int        `json:"followers_count"`
	FriendsCount     int        `json:"friends_count"` // "following" count
	StatusesCount    int        `json:"statuses_count"`
	FavouritesCount  int        `json:"favourites_count,omitempty"`
	ListedCount      int        `json:"listed_count,omitempty"`
	MediaCount       int        `json:"media_count,omitempty"`
	Verified         bool       `json:"verified"`
	Blue             bool       `json:"blue,omitempty"`
	BlueType         string     `json:"blue_type,omitempty"`
	Protected        bool       `json:"protected,omitempty"`
	CreatedAt        string     `json:"created_at"`
	Location         string     `json:"location"`
	URL              string     `json:"url"`
	ProfileImageURL  string     `json:"profile_image_url,omitempty"`
	ProfileBannerURL string     `json:"profile_banner_url,omitempty"`
	PinnedTweetIDs   []string   `json:"pinned_tweet_ids,omitempty"`
	DescriptionLinks []TextLink `json:"description_links,omitempty"`
}

// Tweet is a flat post record. Rich fields (media, entities, nested quote/retweet,
// card) are omitempty so a bare post stays compact.
type Tweet struct {
	RestID              string       `json:"rest_id"`
	AuthorID            string       `json:"author_id,omitempty"`
	UserScreenName      string       `json:"user_screen_name"`
	UserName            string       `json:"user_name"`
	CreatedAt           string       `json:"created_at"`
	Text                string       `json:"text"`
	Lang                string       `json:"lang"`
	ReplyCount          int          `json:"reply_count"`
	RetweetCount        int          `json:"retweet_count"`
	LikeCount           int          `json:"like_count"`
	QuoteCount          int          `json:"quote_count"`
	ViewCount           int          `json:"view_count"`
	BookmarkCount       int          `json:"bookmark_count,omitempty"`
	IsRetweet           bool         `json:"is_retweet"`
	IsQuote             bool         `json:"is_quote"`
	ConversationID      string       `json:"conversation_id,omitempty"`
	InReplyToTweetID    string       `json:"in_reply_to_tweet_id,omitempty"`
	InReplyToUserID     string       `json:"in_reply_to_user_id,omitempty"`
	InReplyToScreenName string       `json:"in_reply_to_screen_name,omitempty"`
	Source              string       `json:"source,omitempty"`
	Hashtags            []string     `json:"hashtags,omitempty"`
	Cashtags            []string     `json:"cashtags,omitempty"`
	Mentions            []UserRef    `json:"mentions,omitempty"`
	Links               []TextLink   `json:"links,omitempty"`
	Media               *Media       `json:"media,omitempty"`
	Card                *Card        `json:"card,omitempty"`
	Place               *Place       `json:"place,omitempty"`
	Coordinates         *Coordinates `json:"coordinates,omitempty"`
	Quoted              *Tweet       `json:"quoted,omitempty"`
	Retweeted           *Tweet       `json:"retweeted,omitempty"`
	CommunityNote       string       `json:"community_note,omitempty"`   // birdwatch_pivot.subtitle.text
	Attribution         *UserRef     `json:"attribution,omitempty"`      // source account of a re-uploaded video
	AttributionLink     string       `json:"attribution_link,omitempty"` // original post path for the attributed video
	URL                 string       `json:"url"`
}

// TextLink is one URL entity: the expanded target, the display text, and the
// t.co shortener used in the raw text.
type TextLink struct {
	URL    string `json:"url"`            // expanded_url
	Text   string `json:"text,omitempty"` // display_url
	TCoURL string `json:"tco_url,omitempty"`
}

// UserRef is a light user reference used for mentions and reply targets.
type UserRef struct {
	RestID     string `json:"rest_id,omitempty"`
	ScreenName string `json:"screen_name"`
	Name       string `json:"name,omitempty"`
}

// Media groups a tweet's attached media by kind.
type Media struct {
	Photos   []MediaPhoto    `json:"photos,omitempty"`
	Videos   []MediaVideo    `json:"videos,omitempty"`
	Animated []MediaAnimated `json:"animated,omitempty"`
}

// MediaPhoto is one still image.
type MediaPhoto struct {
	URL string `json:"url"`
}

// MediaVideo is one video with its best-bitrate variant list.
type MediaVideo struct {
	ThumbnailURL string         `json:"thumbnail_url,omitempty"`
	DurationMS   int            `json:"duration_ms,omitempty"`
	Variants     []MediaVariant `json:"variants,omitempty"`
}

// MediaVariant is one encoded rendition of a video.
type MediaVariant struct {
	ContentType string `json:"content_type,omitempty"`
	Bitrate     int    `json:"bitrate,omitempty"`
	URL         string `json:"url"`
}

// MediaAnimated is one animated GIF (served as a silent video).
type MediaAnimated struct {
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	VideoURL     string `json:"video_url"`
}

// Card is a tweet's attached card (summary link preview or poll).
type Card struct {
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Poll        *Poll  `json:"poll,omitempty"`
}

// Poll is a poll card's options and state.
type Poll struct {
	Options  []PollOption `json:"options"`
	Finished bool         `json:"finished"`
}

// PollOption is one poll choice and its vote count.
type PollOption struct {
	Label string `json:"label"`
	Votes int    `json:"votes"`
}

// Place is a tweet's tagged place.
type Place struct {
	ID          string `json:"id,omitempty"`
	FullName    string `json:"full_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

// Coordinates is a tweet's geo point.
type Coordinates struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

// AccountAbout is the flat AboutAccountQuery result: account origin and identity
// metadata that a normal profile does not carry.
type AccountAbout struct {
	ScreenName           string `json:"screen_name"`
	Name                 string `json:"name"`
	CreatedAt            string `json:"created_at,omitempty"`
	BasedIn              string `json:"based_in,omitempty"` // about_profile.account_based_in
	Source               string `json:"source,omitempty"`   // "Connected via ..."
	AffiliateUsername    string `json:"affiliate_username,omitempty"`
	AffiliateLabel       string `json:"affiliate_label,omitempty"`
	UsernameChanges      int    `json:"username_changes,omitempty"`
	LastUsernameChange   string `json:"last_username_change,omitempty"` // ms epoch as string
	IsIdentityVerified   bool   `json:"is_identity_verified,omitempty"`
	VerifiedSince        string `json:"verified_since,omitempty"`
	OverrideVerifiedYear int    `json:"override_verified_year,omitempty"`
	Suspended            bool   `json:"suspended,omitempty"`
}

// userURL builds the profile URL from the screen_name.
func userURL(screenName string) string {
	if screenName == "" {
		return ""
	}
	return "https://x.com/" + screenName
}

// tweetURL builds the status URL, falling back to the /i/status form when the
// author handle is unknown.
func tweetURL(screenName, restID string) string {
	if restID == "" {
		return ""
	}
	if screenName != "" {
		return fmt.Sprintf("https://x.com/%s/status/%s", screenName, restID)
	}
	return "https://x.com/i/status/" + restID
}

// DirectMessage is one message in a DM conversation.
type DirectMessage struct {
	ID             string   `json:"id"`
	ConversationID string   `json:"conversation_id,omitempty"`
	SenderID       string   `json:"sender_id"`
	RecipientID    string   `json:"recipient_id,omitempty"`
	Text           string   `json:"text"`
	CreatedAt      string   `json:"created_at,omitempty"`
	EditCount      int      `json:"edit_count,omitempty"`
	MediaURLs      []string `json:"media_urls,omitempty"`
}

// Conversation is a DM conversation with its loaded messages.
type Conversation struct {
	ID                    string          `json:"id"`
	Type                  string          `json:"type,omitempty"`
	Name                  string          `json:"name,omitempty"`
	AvatarURL             string          `json:"avatar_url,omitempty"`
	Participants          []string        `json:"participants,omitempty"`
	Trusted               bool            `json:"trusted,omitempty"`
	Muted                 bool            `json:"muted,omitempty"`
	NotificationsDisabled bool            `json:"notifications_disabled,omitempty"`
	HasMore               bool            `json:"has_more,omitempty"`
	LastActivityAt        string          `json:"last_activity_at,omitempty"`
	LastMessageID         string          `json:"last_message_id,omitempty"`
	Messages              []DirectMessage `json:"messages,omitempty"`
}

// Inbox is the DM inbox: conversations plus paging/seen markers.
type Inbox struct {
	Conversations            []Conversation `json:"conversations"`
	Cursor                   string         `json:"cursor,omitempty"`
	LastSeenEventID          string         `json:"last_seen_event_id,omitempty"`
	TrustedLastSeenEventID   string         `json:"trusted_last_seen_event_id,omitempty"`
	UntrustedLastSeenEventID string         `json:"untrusted_last_seen_event_id,omitempty"`
}

// Job is an X Jobs posting.
type Job struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description,omitempty"`
	Location           string      `json:"location,omitempty"`
	JobFunction        string      `json:"job_function,omitempty"`
	FormattedSalary    string      `json:"formatted_salary,omitempty"`
	SalaryMin          int         `json:"salary_min,omitempty"`
	SalaryMax          int         `json:"salary_max,omitempty"`
	SalaryInterval     int         `json:"salary_interval,omitempty"`
	SalaryCurrencyCode string      `json:"salary_currency_code,omitempty"`
	JobPageURL         string      `json:"job_page_url,omitempty"`
	RedirectURL        string      `json:"redirect_url,omitempty"`
	IsFeatured         bool        `json:"is_featured,omitempty"`
	Company            *JobCompany `json:"company,omitempty"`
	User               *JobUser    `json:"user,omitempty"`
}

// JobCompany is the hiring company on a Job.
type JobCompany struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Logo string `json:"logo,omitempty"`
}

// JobUser is the poster account on a Job.
type JobUser struct {
	ID           string `json:"id"`
	UserName     string `json:"user_name"`
	FullName     string `json:"full_name,omitempty"`
	ProfileImage string `json:"profile_image,omitempty"`
	IsVerified   bool   `json:"is_verified,omitempty"`
	VerifiedType string `json:"verified_type,omitempty"`
}

// JobLocation is a location suggestion for a job search.
type JobLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// List is a Twitter List's metadata.
type List struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	CreatedBy       string `json:"created_by,omitempty"` // owner's rest_id
	MemberCount     int    `json:"member_count"`
	SubscriberCount int    `json:"subscriber_count"`
	IsFollowing     bool   `json:"is_following"`
	IsMember        bool   `json:"is_member"`
	Private         bool   `json:"private"`
}

// LiveStreamSource is the playback source of a Space's live audio/video stream.
type LiveStreamSource struct {
	Location              string `json:"location,omitempty"`
	NoRedirectPlaybackURL string `json:"no_redirect_playback_url,omitempty"`
	Status                string `json:"status,omitempty"`
	StreamType            string `json:"stream_type,omitempty"`
}

// LiveStreamStatus is a Space's live stream status, resolved from its media key.
type LiveStreamStatus struct {
	Source             *LiveStreamSource `json:"source,omitempty"`
	SessionID          string            `json:"session_id,omitempty"`
	ChatToken          string            `json:"chat_token,omitempty"`
	LifecycleToken     string            `json:"lifecycle_token,omitempty"`
	ShareURL           string            `json:"share_url,omitempty"`
	ChatPermissionType string            `json:"chat_permission_type,omitempty"`
	MediaKey           string            `json:"media_key,omitempty"`
}
