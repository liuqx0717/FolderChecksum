package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testWalkSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestWalkDirFollowLinks(t *testing.T) {
	rootDir := prepareTestDir(t)
	externalDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(externalDir, "outside"), []byte("external file contents"), 0600); err != nil {
		t.Fatal(err)
	}
	testWalkSymlink(t, "outside", filepath.Join(externalDir, "nested-file"))
	testWalkSymlink(t, filepath.Join(rootDir, "dir1"), filepath.Join(externalDir, "nested-dir"))
	testWalkSymlink(t, "dir1", filepath.Join(rootDir, "alias-dir"))
	testWalkSymlink(t, "alias-dir", filepath.Join(rootDir, "chain-dir"))
	testWalkSymlink(t, "dir2/file1", filepath.Join(rootDir, "chain-file"))
	testWalkSymlink(t, externalDir, filepath.Join(rootDir, "external-dir"))
	testWalkSymlink(t, filepath.Join(externalDir, "outside"), filepath.Join(rootDir, "external-file"))

	expect := []walkRes{
		{"alias-dir/file1", 10},
		{"alias-dir/file2", 10},
		{"chain-dir/file1", 10},
		{"chain-dir/file2", 10},
		{"chain-file", 5},
		{"dir1/file1", 10},
		{"dir1/file2", 10},
		{"dir2/dir1/file1", 10},
		{"dir2/dir1/file2", 10},
		{"dir2/file1", 5},
		{"emptyFile", 0},
		{"external-dir/nested-dir/file1", 10},
		{"external-dir/nested-dir/file2", 10},
		{"external-dir/nested-file", 22},
		{"external-dir/outside", 22},
		{"external-file", 22},
		{"file1", 5},
		{"file2", 5},
	}
	// Shared targets must be visited independently under each alias, with
	// slash-separated paths relative to the requested root and target sizes.
	for _, prefix := range []string{"", "../../", "/dir1/../"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			var actual []walkRes
			mustWalkDir(rootDir, prefix, true, func(relPath string, size int64) {
				actual = append(actual, walkRes{relPath, size})
			})
			verifyWalkRes(t, actual, expect)
		})
	}
}

func TestWalkDirResolvesExplicitRootAndPrefix(t *testing.T) {
	rootDir := prepareTestDir(t)
	rootAlias := filepath.Join(t.TempDir(), "root-alias")
	testWalkSymlink(t, rootDir, rootAlias)
	testWalkSymlink(t, "missing", filepath.Join(rootDir, "broken"))
	testWalkSymlink(t, "file1/child", filepath.Join(rootDir, "broken-file"))
	testWalkSymlink(t, "dir2/dir1", filepath.Join(rootDir, "chain"))
	tests := []struct {
		name   string
		root   string
		prefix string
		want   []walkRes
	}{
		{"file prefix", rootDir, "file1", []walkRes{{"file1", 5}}},
		{"file alias prefix", rootDir, "dir2/file1", []walkRes{{"dir2/file1", 5}}},
		{"folder alias prefix", rootDir, "dir2/dir1", []walkRes{{"dir2/dir1/file1", 10}, {"dir2/dir1/file2", 10}}},
		{"alias within prefix", rootDir, "dir2/dir1/file1", []walkRes{{"dir2/dir1/file1", 10}}},
		{"normalized alias prefix", rootDir, "dir1/../dir2/dir1/file1", []walkRes{{"dir2/dir1/file1", 10}}},
		{"chain prefix", rootDir, "chain/file1", []walkRes{{"chain/file1", 10}}},
		{"root alias", rootAlias, "dir1", []walkRes{{"dir1/file1", 10}, {"dir1/file2", 10}}},
		{"root alias with trailing separator", rootAlias + string(filepath.Separator), "dir1", []walkRes{{"dir1/file1", 10}, {"dir1/file2", 10}}},
		{"alias within root", filepath.Join(rootAlias, "dir1"), "", []walkRes{{"file1", 10}, {"file2", 10}}},
		{"folder alias root", filepath.Join(rootDir, "dir2", "dir1"), "", []walkRes{{"file1", 10}, {"file2", 10}}},
		{"empty folder", rootDir, "dir1/emptyDir", nil},
		{"missing prefix", rootDir, "missing", nil},
		{"missing ancestor", rootDir, "missing/child", nil},
		{"missing descendant", rootDir, "dir1/missing", nil},
		{"regular file ancestor", rootDir, "file1/child", nil},
		{"alias file ancestor", rootDir, "dir2/file1/child", nil},
		{"alias missing descendant", rootDir, "dir2/dir1/missing", nil},
		{"broken prefix", rootDir, "broken", nil},
		{"broken intermediate prefix", rootDir, "broken/child", nil},
		{"broken prefix with regular file ancestor", rootDir, "broken-file", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, followLinks := range []bool{false, true} {
				var actual []walkRes
				mustWalkDir(tt.root, tt.prefix, followLinks, func(relPath string, size int64) {
					actual = append(actual, walkRes{relPath, size})
				})
				verifyWalkRes(t, actual, tt.want)
			}
		})
	}
}

func TestWalkDirDoesNotFollowEncounteredBrokenLinksOrCyclesByDefault(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "file"), []byte("contents"), 0600); err != nil {
		t.Fatal(err)
	}
	testWalkSymlink(t, "missing", filepath.Join(rootDir, "broken"))
	testWalkSymlink(t, ".", filepath.Join(rootDir, "cycle"))
	testWalkSymlink(t, "self", filepath.Join(rootDir, "self"))
	var actual []walkRes
	mustWalkDir(rootDir, "", false, func(relPath string, size int64) {
		actual = append(actual, walkRes{relPath, size})
	})
	verifyWalkRes(t, actual, []walkRes{{"file", 8}})
}

func TestWalkDirFollowLinksOnlyControlsEncounteredLinks(t *testing.T) {
	rootDir := prepareTestDir(t)
	rootAlias := filepath.Join(t.TempDir(), "root-alias")
	testWalkSymlink(t, rootDir, rootAlias)
	testWalkSymlink(t, "dir2", filepath.Join(rootDir, "prefix-alias"))
	if err := os.WriteFile(filepath.Join(rootDir, "dir2", "real-file"), []byte("contents"), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		root   string
		prefix string
		base   string
	}{
		{"root alias", rootAlias, "dir2", "dir2/"},
		{"prefix alias", rootDir, "prefix-alias", "prefix-alias/"},
		{"alias within root", filepath.Join(rootAlias, "dir2"), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, followLinks := range []bool{false, true} {
				want := []walkRes{{tt.base + "real-file", 8}}
				if followLinks {
					want = append([]walkRes{
						{tt.base + "dir1/file1", 10},
						{tt.base + "dir1/file2", 10},
						{tt.base + "file1", 5},
					}, want...)
				}
				var actual []walkRes
				mustWalkDir(tt.root, tt.prefix, followLinks, func(relPath string, size int64) {
					actual = append(actual, walkRes{relPath, size})
				})
				verifyWalkRes(t, actual, want)
			}
		})
	}
}

func TestWalkDirFollowLinksRejectsBrokenLinksAndCycles(t *testing.T) {
	const rootEnv = "FOLDERCHECKSUM_TEST_FOLLOWLINKS_ROOT"
	const prefixEnv = "FOLDERCHECKSUM_TEST_FOLLOWLINKS_PREFIX"
	if rootDir := os.Getenv(rootEnv); rootDir != "" {
		mustWalkDir(rootDir, os.Getenv(prefixEnv), true, func(string, int64) {})
		return
	}

	tests := []struct {
		name      string
		wantError string
		prepare   func(*testing.T, string) string
	}{
		{"broken encountered link", "Failed to follow symlink 'broken':", func(t *testing.T, rootDir string) string {
			testWalkSymlink(t, "missing", filepath.Join(rootDir, "broken"))
			return ""
		}},
		{"link target has regular file ancestor", "Failed to follow symlink 'broken':", func(t *testing.T, rootDir string) string {
			if err := os.WriteFile(filepath.Join(rootDir, "file"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			testWalkSymlink(t, "file/child", filepath.Join(rootDir, "broken"))
			return ""
		}},
		{"self link", "Failed to follow symlink 'loop':", func(t *testing.T, rootDir string) string {
			testWalkSymlink(t, "loop", filepath.Join(rootDir, "loop"))
			return ""
		}},
		{"link chain cycle", "Failed to follow symlink 'first':", func(t *testing.T, rootDir string) string {
			testWalkSymlink(t, "second", filepath.Join(rootDir, "first"))
			testWalkSymlink(t, "first", filepath.Join(rootDir, "second"))
			return ""
		}},
		{"directory to itself", "Directory cycle at 'loop'", func(t *testing.T, rootDir string) string {
			testWalkSymlink(t, ".", filepath.Join(rootDir, "loop"))
			return ""
		}},
		{"directory to ancestor", "Directory cycle at 'child/parent'", func(t *testing.T, rootDir string) string {
			if err := os.Mkdir(filepath.Join(rootDir, "child"), 0755); err != nil {
				t.Fatal(err)
			}
			testWalkSymlink(t, "..", filepath.Join(rootDir, "child", "parent"))
			return ""
		}},
		{"two directory cycle", "Directory cycle at 'first/next/next'", func(t *testing.T, rootDir string) string {
			for _, name := range []string{"first", "second"} {
				if err := os.Mkdir(filepath.Join(rootDir, name), 0755); err != nil {
					t.Fatal(err)
				}
			}
			testWalkSymlink(t, "../second", filepath.Join(rootDir, "first", "next"))
			testWalkSymlink(t, "../first", filepath.Join(rootDir, "second", "next"))
			return ""
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			prefix := tt.prepare(t, rootDir)
			// Cycles must produce a useful failure instead of recursing forever.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWalkDirFollowLinksRejectsBrokenLinksAndCycles$")
			cmd.Env = append(os.Environ(), rootEnv+"="+rootDir, prefixEnv+"="+prefix)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("walk did not reject the broken link or cycle before timeout: %v\n%s", ctx.Err(), output)
			}
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("expected fatal walk error, got %v:\n%s", err, output)
			}
			// Directory cycles must be caught at the first repeated directory,
			// before the OS rejects an ever-growing path of symlinks.
			if !strings.Contains(string(output), "[ERROR] "+tt.wantError) {
				t.Fatalf("expected error %q, got:\n%s", tt.wantError, output)
			}
		})
	}
}
