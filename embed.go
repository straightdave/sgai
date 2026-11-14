package sgai

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var distEmbed embed.FS

// DistFS returns the embedded production build of the React frontend.
func DistFS() (fs.FS, error) {
	return fs.Sub(distEmbed, "web/dist")
}
