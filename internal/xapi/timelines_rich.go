package xapi

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Rich-field parsers split out of timelines.go: media, entities, cards, place,
// coordinates, and source. Each is defensive: it returns a zero value (nil/empty)
// when the source shape is absent, so a bare tweet stays compact.

// ---- entities --------------------------------------------------------------- //

// hashtags reads legacy.entities.<key>[].text (key is "hashtags" or "symbols").
func hashtags(legacy map[string]any, key string) []string {
	var out []string
	for _, h := range asSlice(dig(legacy, "entities", key)) {
		if txt := asString(asMap(h)["text"]); txt != "" {
			out = append(out, txt)
		}
	}
	return out
}

// mentions reads legacy.entities.user_mentions[] into light user references.
func mentions(legacy map[string]any) []UserRef {
	var out []UserRef
	for _, m := range asSlice(dig(legacy, "entities", "user_mentions")) {
		mm := asMap(m)
		if sn := asString(mm["screen_name"]); sn != "" {
			out = append(out, UserRef{
				RestID:     asString(mm["id_str"]),
				ScreenName: sn,
				Name:       asString(mm["name"]),
			})
		}
	}
	return out
}

// entityLinks reads a url-bearing entities block (tweet entities or a user's
// description entities) into TextLinks.
func entityLinks(entities map[string]any) []TextLink {
	var out []TextLink
	for _, u := range asSlice(entities["urls"]) {
		um := asMap(u)
		expanded := asString(um["expanded_url"])
		if expanded == "" {
			expanded = asString(um["url"])
		}
		if expanded == "" {
			continue
		}
		out = append(out, TextLink{
			URL:    expanded,
			Text:   asString(um["display_url"]),
			TCoURL: asString(um["url"]),
		})
	}
	return out
}

// ---- media ------------------------------------------------------------------ //

// parseMedia reads legacy.extended_entities.media[] (falling back to
// legacy.entities.media) and groups it by kind.
func parseMedia(legacy map[string]any) *Media {
	items := asSlice(dig(legacy, "extended_entities", "media"))
	if items == nil {
		items = asSlice(dig(legacy, "entities", "media"))
	}
	if len(items) == 0 {
		return nil
	}
	m := &Media{}
	for _, it := range items {
		mm := asMap(it)
		switch asString(mm["type"]) {
		case "photo":
			m.Photos = append(m.Photos, MediaPhoto{URL: asString(mm["media_url_https"])})
		case "video":
			m.Videos = append(m.Videos, mediaVideo(mm))
		case "animated_gif":
			m.Animated = append(m.Animated, MediaAnimated{
				ThumbnailURL: asString(mm["media_url_https"]),
				VideoURL:     bestVariantURL(mm),
			})
		}
	}
	if len(m.Photos) == 0 && len(m.Videos) == 0 && len(m.Animated) == 0 {
		return nil
	}
	return m
}

func mediaVideo(mm map[string]any) MediaVideo {
	v := MediaVideo{
		ThumbnailURL: asString(mm["media_url_https"]),
		DurationMS:   asInt(dig(mm, "video_info", "duration_millis")),
	}
	for _, va := range asSlice(dig(mm, "video_info", "variants")) {
		vm := asMap(va)
		v.Variants = append(v.Variants, MediaVariant{
			ContentType: asString(vm["content_type"]),
			Bitrate:     asInt(vm["bitrate"]),
			URL:         asString(vm["url"]),
		})
	}
	return v
}

// parseCommunityNote reads the birdwatch_pivot community note text attached to a
// tweet (the "Readers added context" panel). Entity expansion is a render-only
// detail, so the flat text is returned as-is.
func parseCommunityNote(t map[string]any) string {
	return asString(dig(t, "birdwatch_pivot", "subtitle", "text"))
}

// parseAttribution reads the source account of a re-uploaded video from
// legacy.extended_entities.media[].additional_media_info.source_user. When x.com
// shows "video by @other" on a repost, this recovers the original author and the
// original post path.
func parseAttribution(legacy map[string]any) (*UserRef, string) {
	for _, it := range asSlice(dig(legacy, "extended_entities", "media")) {
		mm := asMap(it)
		src := asMap(dig(mm, "additional_media_info", "source_user"))
		if src == nil {
			continue
		}
		ref := &UserRef{
			RestID:     asString(src["id_str"]),
			ScreenName: asString(src["screen_name"]),
			Name:       asString(src["name"]),
		}
		return ref, attributionLink(asString(mm["expanded_url"]))
	}
	return nil, ""
}

// attributionLink turns an expanded_url into the original post path, stripping the
// trailing "/video/1" that x.com appends on the reposting tweet.
func attributionLink(expanded string) string {
	if expanded == "" {
		return ""
	}
	u, err := url.Parse(expanded)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u.Path, "/video/1")
}

// bestVariantURL returns the highest-bitrate variant url (animated gifs carry a
// single mp4 variant).
func bestVariantURL(mm map[string]any) string {
	best, bestRate := "", -1
	for _, va := range asSlice(dig(mm, "video_info", "variants")) {
		vm := asMap(va)
		if r := asInt(vm["bitrate"]); r >= bestRate {
			bestRate, best = r, asString(vm["url"])
		}
	}
	return best
}

// ---- card / poll ------------------------------------------------------------ //

// parseCard reads a tweet card into a summary or poll card, or nil when absent.
func parseCard(card map[string]any) *Card {
	legacy := asMap(card["legacy"])
	name := asString(legacy["name"])
	if name == "" {
		return nil
	}
	bv := bindingValues(legacy)
	if strings.Contains(name, "poll") {
		return &Card{Type: "poll", Poll: parsePoll(bv)}
	}
	return &Card{
		Type:        name,
		Title:       bindingString(bv, "title"),
		Description: bindingString(bv, "description"),
		URL:         cardURL(bv, legacy),
	}
}

// bindingValues flattens card.legacy.binding_values ([]{key,value}) into a map.
func bindingValues(legacy map[string]any) map[string]any {
	out := map[string]any{}
	for _, b := range asSlice(legacy["binding_values"]) {
		bm := asMap(b)
		if k := asString(bm["key"]); k != "" {
			out[k] = asMap(bm["value"])
		}
	}
	return out
}

func bindingString(bv map[string]any, key string) string {
	return asString(asMap(bv[key])["string_value"])
}

func bindingBool(bv map[string]any, key string) bool {
	b, _ := asMap(bv[key])["boolean_value"].(bool)
	return b
}

func cardURL(bv map[string]any, legacy map[string]any) string {
	if u := bindingString(bv, "card_url"); u != "" {
		return u
	}
	if u := bindingString(bv, "website_url"); u != "" {
		return u
	}
	return asString(legacy["url"])
}

// parsePoll reads choiceN_label/choiceN_count bindings into a Poll.
func parsePoll(bv map[string]any) *Poll {
	p := &Poll{Finished: bindingBool(bv, "counts_are_final")}
	for i := 1; i <= 4; i++ {
		label := bindingString(bv, "choice"+strconv.Itoa(i)+"_label")
		if label == "" {
			break
		}
		votes, _ := strconv.Atoi(bindingString(bv, "choice"+strconv.Itoa(i)+"_count"))
		p.Options = append(p.Options, PollOption{Label: label, Votes: votes})
	}
	return p
}

// ---- place / coordinates / source ------------------------------------------ //

// parsePlace reads legacy.place, or nil when absent.
func parsePlace(place map[string]any) *Place {
	if len(place) == 0 {
		return nil
	}
	return &Place{
		ID:          asString(place["id"]),
		FullName:    asString(place["full_name"]),
		Name:        asString(place["name"]),
		Country:     asString(place["country"]),
		CountryCode: asString(place["country_code"]),
	}
}

// parseCoordinates reads legacy.coordinates (GeoJSON [lon, lat]), or nil.
func parseCoordinates(legacy map[string]any) *Coordinates {
	pts := asSlice(dig(legacy, "coordinates", "coordinates"))
	if len(pts) != 2 {
		return nil
	}
	lon, lat := coordFloat(pts[0]), coordFloat(pts[1])
	if lon == 0 && lat == 0 {
		return nil
	}
	return &Coordinates{Longitude: lon, Latitude: lat}
}

func coordFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

var sourceTagRe = regexp.MustCompile(`<[^>]*>`)

// sourceLabel strips the anchor tags from legacy.source, leaving the client name.
func sourceLabel(html string) string {
	if html == "" {
		return ""
	}
	return strings.TrimSpace(sourceTagRe.ReplaceAllString(html, ""))
}

// stringList coerces a JSON []any of strings into []string.
func stringList(v any) []string {
	var out []string
	for _, x := range asSlice(v) {
		if s := asString(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}
