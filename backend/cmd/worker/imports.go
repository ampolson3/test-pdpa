package main

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/platform/files"
)

// fileStore is the files service the import jobs read files and store error reports with.
func fileStore(store *files.S3Store, client *river.Client[pgx.Tx]) *files.Service {
	return &files.Service{Store: store, River: client, Config: files.DefaultConfig()}
}
