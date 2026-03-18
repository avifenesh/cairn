// Package frontend provides an embedded filesystem of the SvelteKit build output.
// The actual embedding only happens with the `embed_frontend` build tag.
// Without the tag, Dist is nil and the server falls back to filesystem reads.
package frontend

import "io/fs"

// Dist holds the embedded frontend static files. Set by embed_prod.go
// when building with -tags embed_frontend.
var Dist fs.FS
