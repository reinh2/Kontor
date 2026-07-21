// Package migrations exposes Kontor's forward-only SQL migrations to the
// application binaries.  Files are checksum-able and travel with the binary.
package migrations

import "embed"

// Files contains every forward-only migration in lexical application order.
//
//go:embed *.sql
var Files embed.FS
