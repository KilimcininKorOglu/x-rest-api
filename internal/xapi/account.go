package xapi

import "net/url"

// Account/profile management. These are REST v1.1 form endpoints (not GraphQL),
// so they hardcode the URL and post form fields via callForm. Profile image and
// banner are sent as base64 directly, without a separate media upload.

// ChangeUsername changes the account's @handle (screen_name).
func (c *XClient) ChangeUsername(username string) error {
	_, err := c.callForm("ChangeUsername", "https://x.com/i/api/1.1/account/settings.json",
		url.Values{"screen_name": {username}})
	return err
}

// UpdateProfile updates the profile fields that are non-nil, leaving the rest
// unchanged (name, url, location, description).
func (c *XClient) UpdateProfile(name, profileURL, location, description *string) error {
	form := url.Values{}
	if name != nil {
		form.Set("name", *name)
	}
	if profileURL != nil {
		form.Set("url", *profileURL)
	}
	if location != nil {
		form.Set("location", *location)
	}
	if description != nil {
		form.Set("description", *description)
	}
	_, err := c.callForm("UpdateProfile", "https://x.com/i/api/1.1/account/update_profile.json", form)
	return err
}

// UpdateProfileImage sets the avatar from a base64-encoded image.
func (c *XClient) UpdateProfileImage(imageBase64 string) error {
	_, err := c.callForm("UpdateProfileImage", "https://x.com/i/api/1.1/account/update_profile_image.json",
		url.Values{"image": {imageBase64}})
	return err
}

// UpdateProfileBanner sets the banner from a base64-encoded image.
func (c *XClient) UpdateProfileBanner(bannerBase64 string) error {
	_, err := c.callForm("UpdateProfileBanner", "https://x.com/i/api/1.1/account/update_profile_banner.json",
		url.Values{"banner": {bannerBase64}})
	return err
}

// ChangePassword changes the account password. On success x.com rotates the
// session, so the ct0/auth_token cookies in use may need to be re-captured.
func (c *XClient) ChangePassword(current, newPassword string) error {
	_, err := c.callForm("ChangePassword", "https://x.com/i/api/i/account/change_password.json",
		url.Values{
			"current_password":      {current},
			"password":              {newPassword},
			"password_confirmation": {newPassword},
		})
	return err
}
