package migrations

import "embed"

// Files contains only trusted, repository-authored migrations.
//
//go:embed *.sql
var Files embed.FS
