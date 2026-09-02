package xapi

import (
	"regexp"
	"strconv"
	"strings"
)

// SearchFilters is a structured x.com search request. BuildSearchQuery renders it
// into the rawQuery operator string. A raw Query is preserved verbatim and each
// operator is appended only when the raw string does not already carry it, so a
// user's own operators are never duplicated.
type SearchFilters struct {
	Query            string // raw search_query, preserved verbatim
	AllWords         []string
	AnyWords         []string
	ExactPhrases     []string
	ExcludeWords     []string
	HashtagsAny      []string
	HashtagsExclude  []string
	FromUsers        []string
	ToUsers          []string
	MentioningUsers  []string
	Lang             string
	TweetType        string // all|originals_only|replies_only|retweets_only|exclude_replies|exclude_retweets
	VerifiedOnly     bool
	BlueVerifiedOnly bool
	HasImages        bool
	HasVideos        bool
	HasLinks         bool
	HasMentions      bool
	HasHashtags      bool
	MinLikes         int
	MinReplies       int
	MinRetweets      int
	Since            string
	Until            string
	Place            string
	Geocode          string
	Near             string
	Within           string
	List             string // list:<id> — restrict to a list's members
	QuotedTweetID    string // quoted_tweet_id:<id> — tweets quoting this tweet
	SinceID          string // since_id:<id> — tweets after this tweet id
	MaxID            string // max_id:<id> — tweets up to this tweet id
}

var handlePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

// Go's RE2 has no lookbehind, so a leading (^|[^A-Za-z0-9_]) group stands in for
// the Python (?<![A-Za-z0-9_]) boundary.
var (
	minOpRe    = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])min_[a-z_]+:`)
	filterOpRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])-?filter:[a-z_]+\b`)
)

func hasOperator(q, name string) bool {
	re := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(name) + `:`)
	return re.MatchString(q)
}

// formatQueryTerm quotes a term that contains whitespace, unless already quoted.
func formatQueryTerm(text string) string {
	v := strings.TrimSpace(text)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		return v
	}
	if strings.ContainsAny(v, " \t\n\r\f\v") {
		return `"` + v + `"`
	}
	return v
}

// queryTimeToken strips a trailing _UTC from a since/until token.
func queryTimeToken(ts string) string {
	t := strings.TrimSpace(ts)
	return strings.TrimSuffix(t, "_UTC")
}

// normalizeHashtag prefixes # unless the value already starts with # or $ (cashtag).
func normalizeHashtag(v string) string {
	t := strings.TrimSpace(v)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "$") {
		return t
	}
	return "#" + t
}

// normalizeHandleList strips @, validates each handle, and dedups case-insensitively.
func normalizeHandleList(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range values {
		h := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		if h == "" || !handlePattern.MatchString(h) {
			continue
		}
		k := strings.ToLower(h)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, h)
	}
	return out
}

// operatorGroup renders name:v terms, OR-grouped when there is more than one.
func operatorGroup(name string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	items := make([]string, 0, len(values))
	for _, v := range values {
		items = append(items, name+":"+v)
	}
	if len(items) == 1 {
		return items[0]
	}
	return "(" + strings.Join(items, " OR ") + ")"
}

var tweetTypeFilters = map[string][]string{
	"originals_only":   {"-filter:replies", "-filter:retweets"},
	"replies_only":     {"filter:replies"},
	"retweets_only":    {"filter:retweets"},
	"exclude_replies":  {"-filter:replies"},
	"exclude_retweets": {"-filter:retweets"},
}

// BuildSearchQuery renders SearchFilters into an x.com rawQuery operator string.
func BuildSearchQuery(f SearchFilters) string {
	q := strings.TrimSpace(f.Query)
	var parts []string
	if q != "" {
		parts = append(parts, q)
	}
	parts = appendWordGroups(parts, f)
	parts = appendHashtags(parts, f)
	parts = appendUserOps(parts, f, q)
	parts = appendFilters(parts, f, q)
	parts = appendMinOps(parts, f, q)
	parts = appendTimeGeo(parts, f, q)
	parts = appendIDsAndList(parts, f, q)
	return strings.TrimSpace(strings.Join(parts, " "))
}

// appendIDsAndList adds the list: and id-range operators (quoted_tweet_id:,
// since_id:, max_id:), skipping any the raw query already carries.
func appendIDsAndList(parts []string, f SearchFilters, q string) []string {
	ids := []struct {
		val, name string
	}{
		{f.List, "list"}, {f.QuotedTweetID, "quoted_tweet_id"},
		{f.SinceID, "since_id"}, {f.MaxID, "max_id"},
	}
	for _, it := range ids {
		if it.val != "" && !hasOperator(q, it.name) {
			parts = append(parts, it.name+":"+strings.TrimSpace(it.val))
		}
	}
	return parts
}

func appendWordGroups(parts []string, f SearchFilters) []string {
	if g := joinTerms(f.AllWords, " AND "); g != "" {
		parts = append(parts, "("+g+")")
	}
	if g := joinTerms(f.AnyWords, " OR "); g != "" {
		parts = append(parts, "("+g+")")
	}
	if g := joinTerms(f.ExactPhrases, " AND "); g != "" {
		parts = append(parts, "("+g+")")
	}
	for _, w := range f.ExcludeWords {
		if t := formatQueryTerm(w); t != "" {
			parts = append(parts, "-"+t)
		}
	}
	return parts
}

// joinTerms formats and joins non-empty terms, returning "" when none remain.
func joinTerms(terms []string, sep string) string {
	var kept []string
	for _, w := range terms {
		if t := formatQueryTerm(w); t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, sep)
}

func appendHashtags(parts []string, f SearchFilters) []string {
	var any []string
	for _, h := range f.HashtagsAny {
		if t := normalizeHashtag(h); t != "" {
			any = append(any, t)
		}
	}
	if len(any) > 0 {
		parts = append(parts, "("+strings.Join(any, " OR ")+")")
	}
	for _, h := range f.HashtagsExclude {
		if t := normalizeHashtag(h); t != "" {
			parts = append(parts, "-"+t)
		}
	}
	return parts
}

func appendUserOps(parts []string, f SearchFilters, q string) []string {
	if !hasOperator(q, "from") {
		if g := operatorGroup("from", normalizeHandleList(f.FromUsers)); g != "" {
			parts = append(parts, g)
		}
	}
	if !hasOperator(q, "to") {
		if g := operatorGroup("to", normalizeHandleList(f.ToUsers)); g != "" {
			parts = append(parts, g)
		}
	}
	mentions := normalizeHandleList(f.MentioningUsers)
	if len(mentions) > 0 {
		items := make([]string, 0, len(mentions))
		for _, u := range mentions {
			items = append(items, "@"+u)
		}
		if len(items) == 1 {
			parts = append(parts, items[0])
		} else {
			parts = append(parts, "("+strings.Join(items, " OR ")+")")
		}
	}
	return parts
}

func appendFilters(parts []string, f SearchFilters, q string) []string {
	if f.Lang != "" && !hasOperator(q, "lang") {
		parts = append(parts, "lang:"+f.Lang)
	}
	if filterOpRe.MatchString(q) {
		return parts
	}
	parts = append(parts, tweetTypeFilters[normTweetType(f.TweetType)]...)
	flags := []struct {
		on   bool
		name string
	}{
		{f.VerifiedOnly, "verified"}, {f.BlueVerifiedOnly, "blue_verified"},
		{f.HasImages, "images"}, {f.HasVideos, "videos"}, {f.HasLinks, "links"},
		{f.HasMentions, "mentions"}, {f.HasHashtags, "hashtags"},
	}
	for _, fl := range flags {
		if fl.on {
			parts = append(parts, "filter:"+fl.name)
		}
	}
	return parts
}

func normTweetType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if _, ok := tweetTypeFilters[t]; ok {
		return t
	}
	return "all"
}

func appendMinOps(parts []string, f SearchFilters, q string) []string {
	if minOpRe.MatchString(q) {
		return parts
	}
	if f.MinLikes > 0 {
		parts = append(parts, "min_faves:"+strconv.Itoa(f.MinLikes))
	}
	if f.MinReplies > 0 {
		parts = append(parts, "min_replies:"+strconv.Itoa(f.MinReplies))
	}
	if f.MinRetweets > 0 {
		parts = append(parts, "min_retweets:"+strconv.Itoa(f.MinRetweets))
	}
	return parts
}

func appendTimeGeo(parts []string, f SearchFilters, q string) []string {
	if f.Since != "" && !hasOperator(q, "since") {
		parts = append(parts, "since:"+queryTimeToken(f.Since))
	}
	if f.Until != "" && !hasOperator(q, "until") {
		parts = append(parts, "until:"+queryTimeToken(f.Until))
	}
	geoTaken := hasOperator(q, "place") || hasOperator(q, "geocode") ||
		hasOperator(q, "near") || hasOperator(q, "within")
	if geoTaken {
		return parts
	}
	switch {
	case f.Place != "":
		parts = append(parts, "place:"+f.Place)
	case f.Geocode != "":
		parts = append(parts, "geocode:"+f.Geocode)
	case f.Near != "":
		parts = append(parts, "near:"+f.Near)
		if f.Within != "" {
			parts = append(parts, "within:"+f.Within)
		}
	case f.Within != "":
		parts = append(parts, "within:"+f.Within)
	}
	return parts
}
