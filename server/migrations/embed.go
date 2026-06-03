// Package migrations embeds the .sql migration files so they ship inside the
// compiled binary (true single-binary self-hosting). The .sql files in this
// directory remain the canonical, human-editable source of the schema.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
