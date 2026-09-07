package main

import "syscall"

func monitorLocalFilesystem(path string) (bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, err
	}
	return stat.Flags&0x00001000 /* MNT_LOCAL, sys/mount.h */ != 0, nil
}
