package local

import (
	"os"
	"path/filepath"
)

// ReadVerification is an opaque, process-local verified prefix. Only a
// successful read-only open can issue it. It never changes the durable marker
// and carries no access rights; every open still checks headers and ownership.
type ReadVerification struct {
	directory string
	identity  string
	epoch     int64
	version   int
	cut       int64
	file      os.FileInfo
}

func (s *Store) ReadVerification() *ReadVerification { return s.readVerification }
func (v *ReadVerification) through(directory string, info StoreInfo, file os.FileInfo, current int64) int64 {
	if v == nil || v.file == nil || file == nil || v.directory != directory || v.identity != info.AuthorityID || v.epoch != info.Epoch || v.version != info.StorageVersion || v.cut > current || !os.SameFile(v.file, file) {
		return 0
	}
	return v.cut
}
func (s *Store) verifiedPrefix(directory string, cut int64) *ReadVerification {
	file, err := os.Stat(filepath.Join(directory, "state.sqlite3"))
	if err != nil {
		return nil
	}
	return &ReadVerification{directory, s.info.AuthorityID, s.info.Epoch, s.info.StorageVersion, cut, file}
}
