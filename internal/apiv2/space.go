package apiv2

import "x-rest-api/internal/xapi"

// SpaceObject renders an Audio Space into a v2 object holding only the selected
// fields. id/state are always present (v2 default set). Timestamps are converted
// from ms epoch to ISO 8601. host_ids is derived from the creator id, the only
// host rest id x.com exposes on the space metadata.
func SpaceObject(sp xapi.Space, sel Selection) map[string]any {
	out := map[string]any{"id": sp.ID, "state": sp.State}
	f := sel.Space
	setStr(out, f, "title", sp.Title)
	setStr(out, f, "creator_id", sp.CreatorID)
	set(out, f, "participant_count", sp.LiveListeners)
	if f["created_at"] {
		if iso := msToISO(sp.CreatedAt); iso != "" {
			out["created_at"] = iso
		}
	}
	if f["started_at"] {
		if iso := msToISO(sp.StartedAt); iso != "" {
			out["started_at"] = iso
		}
	}
	if f["ended_at"] {
		if iso := msToISOString(sp.EndedAt); iso != "" {
			out["ended_at"] = iso
		}
	}
	if f["host_ids"] && sp.CreatorID != "" {
		out["host_ids"] = []string{sp.CreatorID}
	}
	return out
}
