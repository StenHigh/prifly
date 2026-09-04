package local

import (
	"errors"
	"os"
)

// IsPersistenceFailure reports whether an error means this authority could not
// read or write its own storage. Which error types mean that is storage's own
// business, not something every caller should have to know.
func IsPersistenceFailure(err error) bool {
	if err == nil {
		return false
	}
	var path *os.PathError
	return sqliteFailure(err) || errors.As(err, &path) || errors.Is(err, ErrIntegrity) || errors.Is(err, ErrIncompatible)
}

// IsBusy reports whether the storage engine refused a write because another
// writer held the database. That is a wait, not a failure.
func IsBusy(err error) bool { return sqliteBusy(err) }
