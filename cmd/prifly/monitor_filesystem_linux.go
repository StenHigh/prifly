package main

import "syscall"

func monitorLocalFilesystem(path string) (bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, err
	}
	// Remote filesystems and FUSE (which can be remote) are explicitly excluded.
	switch uint64(stat.Type) {
	case 0x6969, 0xff534d42, 0xfe534d42, 0x517b, 0x73757245, 0x5346414f, 0x65735546, 0x01021997:
		return false, nil
	}
	return true, nil
}
