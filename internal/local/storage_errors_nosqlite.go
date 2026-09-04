//go:build !cgo

package local

// Without cgo this build has no SQLite driver at all, so no error can have come
// from one. The authority refuses to open in that build; these answers keep the
// rest of the package readable by tools that do not link it.
func sqliteFailure(error) bool { return false }

func sqliteBusy(error) bool { return false }
