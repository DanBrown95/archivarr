// Package web embeds the built Vue frontend (web/dist) into the Go binary so
// Archivarr ships as a single executable.
//
// A placeholder dist/index.html is committed so `go build` works before the
// frontend is built; `npm run build` (or the Docker build) overwrites dist/
// with the real compiled assets.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded frontend assets rooted at dist/.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
