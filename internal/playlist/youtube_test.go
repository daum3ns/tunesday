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
