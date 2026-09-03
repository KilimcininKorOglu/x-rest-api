package xapi

import (
	"encoding/json"
	"fmt"

	http "github.com/bogdanfinn/fhttp"
)

// hashflagsURL is the REST 1.1 endpoint listing the active hashflags (hashmojis).
const hashflagsURL = "https://x.com/i/api/1.1/hashflags.json"

// Hashflags returns the active hashflags (hashtag emoji images with their display
// windows). The response is a top-level JSON array, so it is decoded directly into
// a slice rather than through the map-based form helpers.
func (c *XClient) Hashflags() ([]Hashflag, error) {
	req, err := http.NewRequest(http.MethodGet, hashflagsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Hashflags: %w", err)
	}
	req.Header = c.sess.headers(c.acct, "en", "")
	body, err := c.doRaw("Hashflags", req)
	if err != nil {
		return nil, err
	}
	var out []Hashflag
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("Hashflags: decode json: %w", err)
	}
	return out, nil
}
