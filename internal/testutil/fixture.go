// Package testutil provides shared helpers for rule tests.
package testutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

// Fixture returns the absolute path to tests/fixtures/<name>, resolving
// relative to the repo root. Use this in package tests so individual
// tests don't have to count "../" levels.
func Fixture(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testutil.Fixture: runtime.Caller failed")
	}
	// thisFile is internal/testutil/fixture.go — repo root is two levels up.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "tests", "fixtures", name)
}
