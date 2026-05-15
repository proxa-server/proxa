// Package web embeds the slim dashboard's static assets and HTML
// templates into the Proxa binary. Per constitution §V, the binary
// ships with everything it needs — no Node.js, no external CDN at
// runtime.
package web

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed all:static
var staticEmbed embed.FS

//go:embed all:templates
var templatesEmbed embed.FS

// StaticFS is the file system rooted at web/static/, exposed for
// the server's /static/* handler.
var StaticFS fs.FS

// Templates is the parsed html/template set rooted at web/templates/.
var Templates *template.Template

func init() {
	sub, err := fs.Sub(staticEmbed, "static")
	if err != nil {
		panic("web: cannot sub static: " + err.Error())
	}
	StaticFS = sub

	Templates = template.Must(template.ParseFS(templatesEmbed, "templates/*.html"))
}
