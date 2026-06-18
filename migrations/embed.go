// Package migrations holds the embedded SQL schema migrations, applied in
// lexical filename order by internal/db at startup.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
