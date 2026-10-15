package migrations

import "embed"

const BaselineVersion = "20261015_200000"

// Files contains the immutable SQL migration history.
//
//go:embed *.sql
var Files embed.FS
