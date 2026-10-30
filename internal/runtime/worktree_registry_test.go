package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// This test replaces PATH to pause Git inside each registry command.
func TestGitWorktreeRegistryLockCoversCommand(t *testing.T) {
	repository := gitRepository(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	started, released := filepath.Join(dir, "started"), filepath.Join(dir, "released")
	for _, path := range []string{started, released} {
		if err := syscall.Mkfifo(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	start, err := os.OpenFile(started, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer start.Close()
	release, err := os.OpenFile(released, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer release.Close()
	shim := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = worktree ]; then\n printf x > '%s'\n read gate < '%s'\n [ \"$2\" != prune ]\n exit $?\nfi\nexec '%s' \"$@\"\n", started, released, realGit)
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(shim), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	lock, err := os.OpenFile(filepath.Join(repository, ".git", "prifly-worktrees.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, operation := range []string{"add", "remove", "prune", "list"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := new(Engine).git(ctx, repository, "worktree", operation)
				done <- err
			}()
			signal := make(chan error, 1)
			go func() {
				var b [1]byte
				_, err := start.Read(b[:])
				signal <- err
			}()
			select {
			case err := <-signal:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("Git did not reach the registry command")
			}
			exclusive := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if exclusive == nil {
				_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
				t.Error("Git registry command ran without its repository lock")
			} else if !errors.Is(exclusive, syscall.EWOULDBLOCK) {
				t.Error(exclusive)
			}
			if _, err := release.WriteString("continue\n"); err != nil {
				t.Fatal(err)
			}
			if err := <-done; (err != nil) != (operation == "prune") {
				t.Errorf("command result: %v", err)
			}
			if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
				t.Fatalf("command leaked registry lock: %v", err)
			}
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		})
	}
}

func TestGitWorktreeRegistryLockUsesCommonDirectory(t *testing.T) {
	t.Parallel()
	e := new(Engine)
	ctx := context.Background()
	repository := gitRepository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if _, err := e.git(ctx, repository, "worktree", "add", "--detach", linked); err != nil {
		t.Fatal(err)
	}
	first, err := e.gitWorktreeRegistryLock(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	other, err := e.gitWorktreeRegistryLock(ctx, gitRepository(t))
	if err != nil {
		t.Fatalf("independent repository was blocked: %v", err)
	}
	other.Close()
	probe, err := os.Open(filepath.Join(repository, ".git", "prifly-worktrees.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("writer does not protect common registry: %v", err)
	}
	wait, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		lock, err := e.gitWorktreeRegistryLock(wait, linked)
		if lock != nil {
			lock.Close()
		}
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled lock wait: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled lock wait did not return")
	}
	first.Close()
	linkedLock, err := e.gitWorktreeRegistryLock(ctx, linked)
	if err != nil {
		t.Fatal(err)
	}
	defer linkedLock.Close()
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("linked tree used a different registry lock: %v", err)
	}
}

func TestGitWorktreeRegistryLockRejectsUnsafeFile(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			repository := gitRepository(t)
			path := filepath.Join(repository, ".git", "prifly-worktrees.lock")
			var err error
			if kind == "symlink" {
				err = os.Symlink(filepath.Join(t.TempDir(), "foreign"), path)
			} else {
				err = syscall.Mkfifo(path, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			lock, err := new(Engine).gitWorktreeRegistryLock(context.Background(), repository)
			if lock != nil {
				lock.Close()
			}
			if err == nil {
				t.Fatalf("accepted %s registry lock", kind)
			}
		})
	}
}
