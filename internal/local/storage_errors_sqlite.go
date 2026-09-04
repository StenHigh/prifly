//go:build cgo

package local

import (
	"errors"

	"github.com/mattn/go-sqlite3"
)

func sqliteFailure(err error) bool {
	var reported sqlite3.Error
	return errors.As(err, &reported)
}

func sqliteBusy(err error) bool {
	var reported sqlite3.Error
	return errors.As(err, &reported) && (reported.Code == sqlite3.ErrBusy || reported.Code == sqlite3.ErrLocked)
}
