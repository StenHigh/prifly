package main

import "testing"

// The monitor serves sealed plans, skill bytes and results. This build cannot
// say who is asking from another machine, so an address reachable from one is
// refused rather than served with a warning.
func TestMonitorListensOnLoopbackOnly(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:7777", "[::1]:7777", "127.0.0.5:1"} {
		if !loopbackOnly(addr) {
			t.Fatalf("a loopback address was refused: %s", addr)
		}
	}
	for _, addr := range []string{"0.0.0.0:7777", "192.168.1.10:7777", "[::]:7777", "example.test:7777", "7777", ""} {
		if loopbackOnly(addr) {
			t.Fatalf("an address reachable from another machine was accepted: %q", addr)
		}
	}
}
