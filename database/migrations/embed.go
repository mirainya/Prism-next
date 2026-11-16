package migrations

import "embed"

const BaselineVersion = "20261026_120000"

// Files contains the immutable SQL migration history.
//
//go:embed *.sql
var Files embed.FS
