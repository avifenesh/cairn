//go:build embed_frontend

package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

func init() {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("frontend embed: " + err.Error())
	}
	Dist = sub
}
