// Package frontend embeds the compiled React/TypeScript assets for desktop delivery.
package frontend

import "embed"

//go:embed all:dist
var DistFS embed.FS
