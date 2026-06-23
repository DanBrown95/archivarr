package util

import "testing"

func TestPathsOverlap(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"/mnt/Media", "/mnt/Media", true},        // identical
		{"/mnt/Media", "/mnt/Media/tv", true},     // child
		{"/mnt", "/mnt/usb1", true},               // parent
		{"/mnt/Media/", "/mnt/Media", true},       // trailing slash normalized
		{"/mnt/Media", "/mnt/MediaBackup", false}, // sibling prefix, not nested
		{"/mnt/Media", "/mnt/usb1", false},        // unrelated
		{"", "/mnt", false},                       // empty guards
		{"/a", "", false},
	}
	for _, c := range cases {
		if got := PathsOverlap(c.a, c.b); got != c.want {
			t.Errorf("PathsOverlap(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
