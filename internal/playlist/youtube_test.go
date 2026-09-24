package playlist

import "testing"

func TestNormalizeYouTubeID(t *testing.T) {
	yt := NewYouTube()
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://youtube.com/watch?v=dQw4w9WgXcQ&ab_channel=Rick", "dQw4w9WgXcQ", true},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://youtu.be/dQw4w9WgXcQ?t=43", "dQw4w9WgXcQ", true},
		{"https://www.youtube.com/shorts/abc123DEF45", "abc123DEF45", true},
		{"http://www.youtube.com/watch?v=badproto", "", false}, // not https
		{"https://example.com/watch?v=dQw4w9WgXcQ", "", false},
		{"not a url", "", false},
	}
	for _, tc := range cases {
		got, ok := yt.NormalizeYouTubeID(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("NormalizeYouTubeID(%q) = %q,%v; want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in                 string
		wantPlatform, want string
		wantID             string
		ok                 bool
	}{
		// YouTube.
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", PlatformYouTube, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://youtube.com/watch?v=dQw4w9WgXcQ&ab_channel=Rick", PlatformYouTube, "https://youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ", PlatformYouTube, "https://music.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://youtu.be/dQw4w9WgXcQ", PlatformYouTube, "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"https://www.youtube.com/shorts/abc123DEF45", PlatformYouTube, "https://www.youtube.com/shorts/abc123DEF45", "abc123DEF45", true},
		// SoundCloud.
		{"https://soundcloud.com/rick-astley-official/never-gonna-give-you-up", PlatformSoundCloud, "https://soundcloud.com/rick-astley-official/never-gonna-give-you-up", "", true},
		{"https://www.soundcloud.com/user/track", PlatformSoundCloud, "https://www.soundcloud.com/user/track", "", true},
		{"https://soundcloud.com/user/track?utm_source=clipboard&utm_medium=text", PlatformSoundCloud, "https://soundcloud.com/user/track?utm_source=clipboard", "", true},
		{"https://soundcloud.com/sets/mix", "", "", "", false}, // playlist rejected
		{"https://soundcloud.com/user", "", "", "", false},     // no track
		{"https://soundcloud.com", "", "", "", false},          // bare host
		{"https://soundcloud.com/user/", "", "", "", false},    // empty track segment
		// Bandcamp.
		{"https://gasolinelollipops.bandcamp.com/track/love-is-free-single", PlatformBandcamp, "https://gasolinelollipops.bandcamp.com/track/love-is-free-single", "", true},
		{"https://x.bandcamp.com/album/resurrection", "", "", "", false}, // album rejected
		{"https://x.bandcamp.com", "", "", "", false},                    // bare host
		{"https://bandcamp.com", "", "", "", false},                      // no artist subdomain
		// Denied.
		{"http://www.youtube.com/watch?v=dQw4w9WgXcQ", "", "", "", false},
		{"https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT", "", "", "", false},
		{"https://vimeo.com/123", "", "", "", false},
		{"https://example.com/watch?v=dQw4w9WgXcQ", "", "", "", false},
		{"not a url", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, tc := range cases {
		got, ok := Normalize(tc.in)
		if ok != tc.ok {
			t.Errorf("Normalize(%q) ok = %v; want %v", tc.in, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Platform != tc.wantPlatform || got.URL != tc.want || got.ID != tc.wantID {
			t.Errorf("Normalize(%q) = %+v; want platform=%q url=%q id=%q", tc.in, got, tc.wantPlatform, tc.want, tc.wantID)
		}
	}
}

func TestStripTrackingParams(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.youtube.com/watch?v=yMR45cZbvDw&list=RDyMR45cZbvDw&start_radio=1&pp=ygURYWx…", "https://www.youtube.com/watch?v=yMR45cZbvDw"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&ab_channel=Rick", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{"https://music.youtube.com/watch?v=yMVwhtEoXd0&si=L19PJjv9TJyGTrbh", "https://music.youtube.com/watch?v=yMVwhtEoXd0"},
		{"https://youtu.be/dQw4w9WgXcQ?t=43", "https://youtu.be/dQw4w9WgXcQ?t=43"}, // no '&', unchanged
		{"abc&def&ghi", "abc"},
		{"noampersand", "noampersand"},
	}
	for _, tc := range cases {
		if got := StripTrackingParams(tc.in); got != tc.want {
			t.Errorf("StripTrackingParams(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}
