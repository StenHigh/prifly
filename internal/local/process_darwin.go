//go:build darwin

package local

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// Darwin's proc_info(2), exposed by libproc, supplies kernel birth times and
// bounded process-group membership without parsing ps output. The fixed ABI is
// proc_bsdinfo in the macOS SDK's sys/proc_info.h (64-bit Darwin).
// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/proc_info.h
type darwinProcessInfo struct {
	Flags, Status, ExitStatus, PID, PPID uint32
	UID, GID, RUID, RGID, SVUID, SVGID   uint32
	Reserved                             uint32
	Comm                                 [16]byte
	Name                                 [32]byte
	Files, PGID, JobCount, TDev, TPGID   uint32
	Nice                                 int32
	StartSeconds, StartMicroseconds      uint64
}

func processBootID() (string, error) {
	return syscall.Sysctl("kern.bootsessionuuid")
}

func readProcess(pid int) (processRecord, error) {
	if pid <= 0 {
		return processRecord{}, errors.New("invalid process id")
	}
	var info darwinProcessInfo
	if unsafe.Sizeof(info) != 136 {
		return processRecord{}, errors.New("unsupported Darwin process ABI")
	}
	// SYS_PROC_INFO=336, PROC_INFO_CALL_PIDINFO=2, PROC_PIDTBSDINFO=3.
	// arg=1 includes the unreaped zombie: arg=0 would turn a normal exit into
	// an apparent loss of identity and lose the group-reuse safety boundary.
	n, _, errno := syscall.Syscall6(336, 2, uintptr(pid), 3, 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	runtime.KeepAlive(&info)
	if errno != 0 {
		return processRecord{}, errno
	}
	if n != unsafe.Sizeof(info) || info.PID != uint32(pid) {
		return processRecord{}, errors.New("incomplete Darwin process identity")
	}
	return processRecord{PID: pid, PPID: int(info.PPID), PGID: int(info.PGID), UID: int(info.UID), StartID: fmt.Sprintf("%d.%06d", info.StartSeconds, info.StartMicroseconds), Zombie: info.Status == 5}, nil
}

func readProcessGroup(pgid int) ([]processRecord, error) {
	if pgid <= 1 {
		return nil, errors.New("invalid process group")
	}
	// A full buffer is not evidence of a complete group snapshot. Refuse it;
	// detached descendants remain forbidden by the cooperative runner contract.
	pids := make([]int32, 16384)
	// SYS_PROC_INFO=336, PROC_INFO_CALL_LISTPIDS=1, PROC_PGRP_ONLY=2.
	n, _, errno := syscall.Syscall6(336, 1, 2, uintptr(pgid), 0, uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)*4))
	runtime.KeepAlive(pids)
	if errno != 0 {
		return nil, errno
	}
	if n%4 != 0 || n >= uintptr(len(pids)*4) {
		return nil, errors.New("process group exceeds observation capacity")
	}
	group := make([]processRecord, 0, int(n)/4)
	for _, pid := range pids[:int(n)/4] {
		if pid <= 0 {
			continue
		}
		p, err := readProcess(int(pid))
		if errors.Is(err, syscall.ESRCH) {
			continue // exited between the group list and its identity observation
		}
		if err != nil {
			return nil, err
		}
		if p.PGID == pgid {
			group = append(group, p)
		}
	}
	return group, nil
}
