// Package migrations embeds the SQL migrations so the binary can apply them
// without the files being present at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
