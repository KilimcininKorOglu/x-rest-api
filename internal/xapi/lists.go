package xapi

import "fmt"

// List parsing. A List can arrive as a standalone `data.list` (ListByRestId,
// UpdateList) or as timeline entries whose itemContent carries a `list` object
// (ListsManagementPageTimeline). Both feed parseList.

// ListInfo returns a single list's metadata (ListByRestId).
func (c *XClient) ListInfo(id string) (*List, error) {
	payload, err := c.call("ListByRestId", map[string]any{"listId": id})
	if err != nil {
		return nil, err
	}
	l := parseList(asMap(dig(payload, "data", "list")))
	if l == nil {
		return nil, fmt.Errorf("ListInfo: %s", responseErr(payload))
	}
	return l, nil
}

// parseList builds a List from a raw list object, or nil when it has no id.
func parseList(raw map[string]any) *List {
	if raw == nil {
		return nil
	}
	id := asString(raw["id_str"])
	if id == "" {
		return nil
	}
	return &List{
		ID:              id,
		Name:            asString(raw["name"]),
		Description:     asString(raw["description"]),
		CreatedAt:       epochToISO(raw["created_at"]),
		CreatedBy:       asString(dig(raw, "user_results", "result", "rest_id")),
		MemberCount:     asInt(raw["member_count"]),
		SubscriberCount: asInt(raw["subscriber_count"]),
		IsFollowing:     asBool(raw["following"]),
		IsMember:        asBool(raw["is_member"]),
		Private:         asString(raw["mode"]) == "Private",
	}
}

// parseListsTimeline returns the lists and bottom cursor from a
// ListsManagementPageTimeline response; the itemContent carries a `list` object.
func parseListsTimeline(payload map[string]any) ([]List, string) {
	es := entries(payload)
	var lists []List
	for _, it := range itemContents(es) {
		if l := parseList(asMap(it["list"])); l != nil {
			lists = append(lists, *l)
		}
	}
	return lists, cursor(es, "Bottom")
}

// listKey dedups lists by id across paginated pages.
func listKey(l List) string { return l.ID }
