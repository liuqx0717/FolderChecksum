package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func testConfig(t *testing.T, f *flags) *config {
	t.Helper()
	previousLogLevel := logLevel
	defer func() { logLevel = previousLogLevel }()
	cfg := flagsToConfig(f)
	t.Cleanup(func() { cfg.db.Close() })
	return cfg
}

func TestDatabaseExclusionCaseSensitive(t *testing.T) {
	rootDir := t.TempDir()
	// Existing files with different casing must not change the exclusions,
	// regardless of whether the filesystem treats these names as aliases.
	for _, name := range []string{"CHECKSUM.db", "CHECKSUM.db-wal", "CHECKSUM.db-shm"} {
		if err := os.WriteFile(filepath.Join(rootDir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := testConfig(t, &flags{dbFile: "checksum.db", checksum: "auto", rootDir: rootDir, j: 1})
	for _, name := range []string{"checksum.db", "checksum.db-wal", "checksum.db-shm"} {
		if !shouldExcludePath(cfg, name) {
			t.Errorf("specified database file %q is not excluded", name)
		}
	}
	for _, name := range []string{
		"CHECKSUM.db", "CHECKSUM.db-wal", "CHECKSUM.db-shm",
		"checksumXdb", "subdir/checksum.db",
	} {
		if shouldExcludePath(cfg, name) {
			t.Errorf("nonmatching path %q is excluded", name)
		}
	}

	// Explicit includes still override automatic database exclusions.
	cfg.includeRe = getRegexFromList([]string{regexp.QuoteMeta("checksum.db")})
	if shouldExcludePath(cfg, "checksum.db") {
		t.Error("explicit include did not override the database exclusion")
	}
}

func TestDatabaseExclusionExplicitPath(t *testing.T) {
	rootDir := t.TempDir()
	// A database path containing a separator is not automatically excluded,
	// even when it points to a file inside the scanned root directory.
	cfg := testConfig(t, &flags{dbFile: filepath.Join(rootDir, "checksum.db"), checksum: "auto", rootDir: rootDir, j: 1})
	if shouldExcludePath(cfg, "checksum.db") {
		t.Error("explicit database path was automatically excluded")
	}
}
