package apiv2

import "x-rest-api/internal/xapi"

// ListObject renders a list into a v2 object holding only the selected fields.
// id/name are always present (v2 default set).
func ListObject(l xapi.List, sel Selection) map[string]any {
	out := map[string]any{"id": l.ID, "name": l.Name}
	f := sel.List
	setStr(out, f, "description", l.Description)
	setStr(out, f, "owner_id", l.CreatedBy)
	set(out, f, "private", l.Private)
	set(out, f, "member_count", l.MemberCount)
	set(out, f, "follower_count", l.SubscriberCount)
	if f["created_at"] {
		if iso := toISO8601(l.CreatedAt); iso != "" {
			out["created_at"] = iso
		}
	}
	return out
}
