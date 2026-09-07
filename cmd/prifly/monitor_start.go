package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const monitorAddress = "127.0.0.1:7777"

var ensureRunMonitor = startRunMonitor

func (c *cli) openWithMonitor(root string, readOnly bool) (*prifly.Engine, error) {
	e, err := prifly.Open(root, readOnly)
	if err == nil && !readOnly {
		e.AfterRunCreated = func() {
			if err := ensureRunMonitor(e.Root); err != nil {
				fmt.Fprintf(c.errout, "monitor: %v; Run continues. Start manually with prifly monitor.\n", err)
			}
		}
	}
	return e, err
}
func monitorReady(addr string) bool {
	client := http.Client{Timeout: 150 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var health struct {
		Service string `json:"service"`
		UID     int    `json:"uid"`
	}
	return resp.StatusCode == 200 && json.NewDecoder(resp.Body).Decode(&health) == nil && health.Service == "prifly-local-monitor/1" && health.UID == os.Geteuid()
}
func startRunMonitor(root string) error {
	if err := registerMonitorRoot(root); err != nil {
		return err
	}
	if monitorReady(monitorAddress) {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	_, err = spawnMonitor(executable, root, monitorAddress, "")
	return err
}
func spawnMonitor(executable, root, addr, scanRoot string) (*os.Process, error) {
	dir, err := monitorDirectory()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	args := []string{"--project", root, "monitor", "--addr", addr}
	if scanRoot != "" {
		args = append(args, "--scan-root", scanRoot)
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = root
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if monitorReady(addr) {
			return cmd.Process, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cmd.Process, errors.New("local monitor did not answer; check its port and user-config Pri-Fly/monitor/server.log")
}
