//go:build linux

package local

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func processBootID() (string, error) {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", errors.New("empty Linux boot identity")
	}
	return id, nil
}

func readProcess(pid int) (processRecord, error) {
	if pid <= 0 {
		return processRecord{}, errors.New("invalid process id")
	}
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return processRecord{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return processRecord{}, err
	}
	// comm may itself contain spaces and ')' characters; subsequent fields
	// follow the final ')' (proc_pid_stat(5)). starttime is field 22 in clock ticks.
	close := strings.LastIndexByte(string(data), ')')
	if close < 0 {
		return processRecord{}, errors.New("invalid Linux process stat")
	}
	fields := strings.Fields(string(data[close+1:]))
	if len(fields) < 20 {
		return processRecord{}, errors.New("incomplete Linux process stat")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return processRecord{}, err
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return processRecord{}, err
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return processRecord{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return processRecord{}, errors.New("Linux process uid unavailable")
	}
	return processRecord{PID: pid, PPID: ppid, PGID: pgid, UID: int(stat.Uid), StartID: fields[19], Zombie: fields[0] == "Z" || fields[0] == "X"}, nil
}

func readProcessGroup(pgid int) ([]processRecord, error) {
	if pgid <= 1 {
		return nil, errors.New("invalid process group")
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	// ponytail: /proc scanning is sufficient for one foreground slot; replace
	// with a qualified cgroup observer when the parallel runner is introduced.
	if len(entries) > 131072 {
		return nil, errors.New("process table exceeds observation capacity")
	}
	var group []processRecord
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		p, err := readProcess(pid)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return nil, err // do not claim a complete snapshot if procfs hid a member
		}
		if p.PGID == pgid {
			group = append(group, p)
		}
	}
	return group, nil
}
