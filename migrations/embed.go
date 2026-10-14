// Package migrations embeds the SQL schema.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
