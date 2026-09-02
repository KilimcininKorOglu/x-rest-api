package apiv2

import "x-rest-api/internal/xapi"

// set assigns a value when the named field is selected.
func set(out map[string]any, f map[string]bool, key string, val any) {
	if f[key] {
		out[key] = val
	}
}

// setStr assigns a string value when the field is selected and the value is
// non-empty.
func setStr(out map[string]any, f map[string]bool, key, val string) {
	if f[key] && val != "" {
		out[key] = val
	}
}

// ref is one referenced_tweets entry.
func ref(kind, id string) map[string]any {
	return map[string]any{"type": kind, "id": id}
}

// tags renders hashtag/cashtag strings as v2 entity objects ([{tag}]).
func tags(vals []string) []map[string]any {
	out := make([]map[string]any, 0, len(vals))
	for _, v := range vals {
		out = append(out, map[string]any{"tag": v})
	}
	return out
}

// mentionEntities renders mentions as v2 entity objects, attaching the numeric id
// when known.
func mentionEntities(ms []xapi.UserRef) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		e := map[string]any{"username": m.ScreenName}
		if m.RestID != "" {
			e["id"] = m.RestID
		}
		out = append(out, e)
	}
	return out
}

// urlEntities renders link entities into the v2 url shape.
func urlEntities(links []xapi.TextLink) []map[string]any {
	out := make([]map[string]any, 0, len(links))
	for _, l := range links {
		e := map[string]any{"expanded_url": l.URL}
		if l.TCoURL != "" {
			e["url"] = l.TCoURL
		}
		if l.Text != "" {
			e["display_url"] = l.Text
		}
		out = append(out, e)
	}
	return out
}
