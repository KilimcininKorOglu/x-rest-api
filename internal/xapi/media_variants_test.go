package xapi

import "testing"

// TestBestVideoURL covers the ranking bestVideoURL applies: the highest-bitrate
// mp4 wins, and the HLS playlist only answers when x.com offers no mp4.
func TestBestVideoURL(t *testing.T) {
	const hls = "https://v/playlist.m3u8"
	cases := []struct {
		name     string
		variants []MediaVariant
		want     string
	}{
		{
			name: "highest bitrate mp4 wins",
			variants: []MediaVariant{
				{ContentType: "video/mp4", Bitrate: 256000, URL: "https://v/lo.mp4"},
				{ContentType: "video/mp4", Bitrate: 832000, URL: "https://v/hi.mp4"},
				{ContentType: "video/mp4", Bitrate: 432000, URL: "https://v/mid.mp4"},
			},
			want: "https://v/hi.mp4",
		},
		{
			name: "mp4 beats the playlist even though the playlist has no bitrate",
			variants: []MediaVariant{
				{ContentType: "application/x-mpegURL", URL: hls},
				{ContentType: "video/mp4", Bitrate: 256000, URL: "https://v/lo.mp4"},
			},
			want: "https://v/lo.mp4",
		},
		{
			name:     "playlist answers when there is no mp4",
			variants: []MediaVariant{{ContentType: "application/x-mpegURL", URL: hls}},
			want:     hls,
		},
		{
			name: "a variant with no url is skipped",
			variants: []MediaVariant{
				{ContentType: "video/mp4", Bitrate: 999000},
				{ContentType: "video/mp4", Bitrate: 256000, URL: "https://v/lo.mp4"},
			},
			want: "https://v/lo.mp4",
		},
		{name: "no variants", variants: nil, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bestVideoURL(c.variants); got != c.want {
				t.Errorf("bestVideoURL = %q, want %q", got, c.want)
			}
		})
	}
}
