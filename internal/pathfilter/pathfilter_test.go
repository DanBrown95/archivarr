package pathfilter_test

import (
	"testing"

	"github.com/danbrown95/archivarr/internal/pathfilter"
)

func TestSkip(t *testing.T) {
	cases := []struct {
		name string
		r    pathfilter.Rules
		rel  string
		want bool
	}{
		{"no rules keeps everything", pathfilter.Rules{}, "Movies/a.mkv", false},
		{"exclude by extension glob", pathfilter.Rules{Exclude: []string{"*.tmp"}}, "x/y.tmp", true},
		{"extension glob is case-insensitive", pathfilter.Rules{Exclude: []string{"*.tmp"}}, "x/Y.TMP", true},
		{"bare extension does not match a real file", pathfilter.Rules{Exclude: []string{".tmp"}}, "movie.tmp", false},
		{"exclude by exact filename", pathfilter.Rules{Exclude: []string{".DS_Store"}}, "dir/.ds_store", true},
		{"exclude matches a path segment (dir)", pathfilter.Rules{Exclude: []string{"@eaDir"}}, "@eaDir/thumb.jpg", true},
		{"qbittorrent partial mixed case", pathfilter.Rules{Exclude: []string{"*.!qb"}}, "f.!qB", true},
		{"non-matching exclude keeps file", pathfilter.Rules{Exclude: []string{"*.tmp"}}, "a.mkv", false},
		{"include-ext keeps listed", pathfilter.Rules{IncludeExt: []string{"mkv"}}, "a.MKV", false},
		{"include-ext drops others", pathfilter.Rules{IncludeExt: []string{"mkv"}}, "a.jpg", true},
		{"include-ext with dot in config", pathfilter.Rules{IncludeExt: []string{".mkv"}}, "a.mkv", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.r.Skip(c.rel); got != c.want {
				t.Fatalf("Skip(%q) = %v, want %v", c.rel, got, c.want)
			}
		})
	}
}
