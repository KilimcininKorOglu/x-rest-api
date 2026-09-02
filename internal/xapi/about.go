package xapi

// AboutAccountQuery flat parsing. The response hangs under
// data.user_result_by_screen_name.result, not the standard timeline envelope, so
// it has its own parser.

// parseAboutAccount turns an AboutAccountQuery response into an AccountAbout.
func parseAboutAccount(payload map[string]any) *AccountAbout {
	user := asMap(dig(payload, "data", "user_result_by_screen_name", "result"))
	if user == nil {
		return nil
	}
	if asString(user["unavailable_reason"]) == "Suspended" {
		return &AccountAbout{Suspended: true}
	}
	core := asMap(user["core"])
	a := &AccountAbout{
		ScreenName:     asString(core["screen_name"]),
		Name:           asString(core["name"]),
		CreatedAt:      asString(core["created_at"]),
		AffiliateLabel: asString(dig(user, "identity_profile_labels_highlighted_label", "label", "description")),
	}
	if about := asMap(user["about_profile"]); about != nil {
		a.BasedIn = asString(about["account_based_in"])
		a.Source = asString(about["source"])
		a.AffiliateUsername = asString(about["affiliate_username"])
		a.UsernameChanges = asInt(dig(about, "username_changes", "count")) // count is a string; asInt coerces it
		a.LastUsernameChange = asString(dig(about, "username_changes", "last_changed_at_msec"))
	}
	if info := asMap(user["verification_info"]); info != nil {
		a.IsIdentityVerified, _ = info["is_identity_verified"].(bool)
		if reason := asMap(info["reason"]); reason != nil {
			a.OverrideVerifiedYear = asInt(reason["override_verified_year"])
			a.VerifiedSince = asString(reason["verified_since_msec"])
		}
	}
	return a
}

// UserAbout fetches the flat AboutAccountQuery result for a handle.
func (c *XClient) UserAbout(handle string) (*AccountAbout, error) {
	payload, err := c.call("AboutAccountQuery", map[string]any{"screenName": normalizeHandle(handle)})
	if err != nil {
		return nil, err
	}
	return parseAboutAccount(payload), nil
}
