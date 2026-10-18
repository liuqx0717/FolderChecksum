package main

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func followLinksCLISymlink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Cannot create symlinks: %s", err)
		}
		t.Fatal(err)
	}
}

func TestFollowLinksCLI(t *testing.T) {
	for _, tc := range []struct {
		name        string
		followLinks bool
		sizeOnly    bool
	}{
		{name: "default_skips_links"},
		{name: "checksums", followLinks: true},
		{name: "size_only", followLinks: true, sizeOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()
			dbFile := filepath.Join(t.TempDir(), "checksums.db")
			target := filepath.Join(rootDir, "real", "sample.txt")
			contents := []byte("original target contents\n")
			if err := os.Mkdir(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, contents, 0600); err != nil {
				t.Fatal(err)
			}
			for _, link := range []struct{ target, name string }{
				{filepath.Join("real", "sample.txt"), "file-link.txt"},
				{"real", "directory-link"},
			} {
				followLinksCLISymlink(t, link.target, filepath.Join(rootDir, link.name))
			}

			paths := []string{"real/sample.txt"}
			args := []string{"-j", "1", "-checksum", "md5", "-dbfile", dbFile}
			if tc.followLinks {
				args = append(args, "-followlinks")
				paths = append(paths, "file-link.txt", "directory-link/sample.txt")
			}
			if tc.sizeOnly {
				args = append(args, "-sizeonly")
			}
			sort.Strings(paths)

			run := func(update bool, status string) {
				t.Helper()
				callArgs := append([]string(nil), args...)
				if update {
					callArgs = append(callArgs, "-update")
				}
				stdout, stderr, err := runChecksumCLI(t, append(callArgs, rootDir)...)
				if err != nil {
					t.Fatalf("CLI failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
				}
				var expected []string
				if status != "" {
					for _, path := range paths {
						expected = append(expected, status+": "+path)
					}
				}
				actual := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
				sort.Strings(actual)
				if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
					t.Fatalf("update=%t: got output %q, want %q", update, stdout, expected)
				}
			}
			verifyStored := func(data []byte) {
				t.Helper()
				db := mustOpenDb(dbFile)
				defer db.Close()
				var checksum any
				if !tc.sizeOnly {
					checksum = fmt.Sprintf("%x", md5.Sum(data))
				}
				var expected []fileRow
				for _, path := range paths {
					expected = append(expected, fileRow{
						path: path, size: int64(len(data)), checksum: checksum,
					})
				}
				verifyFileRows(t, getAllRowsFromFiles(t, db), expected)
			}

			run(true, "new")
			verifyStored(contents)
			run(true, "")
			verifyStored(contents)

			// Keep the byte count for checksum mode so changes cannot be
			// detected from size alone; size-only mode needs a larger target.
			changed := []byte(strings.ToUpper(string(contents)))
			if tc.sizeOnly {
				changed = append(changed, []byte("more contents\n")...)
			}
			if err := os.WriteFile(target, changed, 0600); err != nil {
				t.Fatal(err)
			}
			run(false, "changed")
			verifyStored(contents)
			run(true, "changed")
			verifyStored(changed)
			run(false, "")
		})
	}
}

func followLinksCLICheckScan(t *testing.T, rootDir, prefix, mode string, contents []byte, paths []string) {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "checksums.db")
	args := []string{"-j", "1", "-checksum", "md5", "-dbfile", dbFile, "-update"}
	if mode != "default" {
		args = append(args, "-followlinks="+mode)
	}
	args = append(args, rootDir)
	if prefix != "" {
		args = append(args, prefix)
	}
	stdout, stderr, err := runChecksumCLI(t, args...)
	if err != nil {
		t.Fatalf("CLI failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var expected []fileRow
	var output []string
	for _, path := range paths {
		output = append(output, "new: "+path)
		expected = append(expected, fileRow{
			path: path, size: int64(len(contents)), checksum: fmt.Sprintf("%x", md5.Sum(contents)),
		})
	}
	sort.Strings(output)
	actual := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	sort.Strings(actual)
	if strings.Join(actual, "\n") != strings.Join(output, "\n") {
		t.Errorf("got output %q, want %q", stdout, output)
	}
	db := mustOpenDb(dbFile)
	defer db.Close()
	verifyFileRows(t, getAllRowsFromFiles(t, db), copyAndSortFileRows(expected))
}

func TestFollowLinksCLIExplicitPrefix(t *testing.T) {
	rootDir := t.TempDir()
	target := filepath.Join(rootDir, "real", "nested", "sample.txt")
	contents := []byte("target file contents\n")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, contents, 0600); err != nil {
		t.Fatal(err)
	}
	followLinksCLISymlink(t, filepath.Join("real", "nested", "sample.txt"), filepath.Join(rootDir, "file-link.txt"))
	followLinksCLISymlink(t, "real", filepath.Join(rootDir, "directory-link"))
	followLinksCLISymlink(t, "sample.txt", filepath.Join(rootDir, "real", "nested", "nested-link.txt"))
	followLinksCLISymlink(t, "nested", filepath.Join(rootDir, "real", "nested-directory-link"))
	for _, mode := range []string{"default", "false", "true"} {
		for _, tc := range []struct {
			prefix      string
			paths       []string
			linkedPaths []string
		}{
			{prefix: "file-link.txt", paths: []string{"file-link.txt"}},
			{
				prefix: "directory-link", paths: []string{"directory-link/nested/sample.txt"},
				linkedPaths: []string{
					"directory-link/nested/nested-link.txt",
					"directory-link/nested-directory-link/sample.txt",
					"directory-link/nested-directory-link/nested-link.txt",
				},
			},
			{
				prefix: "directory-link/nested", paths: []string{"directory-link/nested/sample.txt"},
				linkedPaths: []string{"directory-link/nested/nested-link.txt"},
			},
			{prefix: "directory-link/nested/sample.txt", paths: []string{"directory-link/nested/sample.txt"}},
			{prefix: "directory-link/nested/nested-link.txt", paths: []string{"directory-link/nested/nested-link.txt"}},
		} {
			t.Run(mode+"/"+tc.prefix, func(t *testing.T) {
				paths := append([]string(nil), tc.paths...)
				if mode == "true" {
					paths = append(paths, tc.linkedPaths...)
				}
				followLinksCLICheckScan(t, rootDir, tc.prefix, mode, contents, paths)
			})
		}
	}
}

func TestFollowLinksCLIExplicitRoot(t *testing.T) {
	parentDir := t.TempDir()
	realRoot := filepath.Join(parentDir, "real")
	contents := []byte("root target contents\n")
	if err := os.Mkdir(realRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "sample.txt"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	followLinksCLISymlink(t, "real", filepath.Join(parentDir, "root-link"))
	followLinksCLISymlink(t, ".", filepath.Join(parentDir, "parent-link"))
	followLinksCLISymlink(t, "sample.txt", filepath.Join(realRoot, "nested-link.txt"))
	for _, mode := range []string{"default", "false", "true"} {
		for _, root := range []string{"root-link", "root-link/", "root-link/.", "parent-link/real"} {
			t.Run(mode+"/"+root, func(t *testing.T) {
				paths := []string{"sample.txt"}
				if mode == "true" {
					paths = append(paths, "nested-link.txt")
				}
				// Preserve trailing separators in the actual CLI argument.
				rootArg := parentDir + string(os.PathSeparator) + filepath.FromSlash(root)
				followLinksCLICheckScan(t, rootArg, "", mode, contents, paths)
			})
		}
	}
}

func TestFollowLinksCLIBrokenLinks(t *testing.T) {
	rootDir := t.TempDir()
	followLinksCLISymlink(t, "missing", filepath.Join(rootDir, "broken-link"))
	for _, mode := range []string{"default", "false", "true"} {
		for _, prefix := range []string{"", "broken-link", "broken-link/child"} {
			t.Run(mode+"/prefix="+prefix, func(t *testing.T) {
				dbFile := filepath.Join(t.TempDir(), "checksums.db")
				args := []string{"-j", "1", "-dbfile", dbFile, "-update"}
				if mode != "default" {
					args = append(args, "-followlinks="+mode)
				}
				args = append(args, rootDir)
				if prefix != "" {
					args = append(args, prefix)
				}
				stdout, stderr, err := runChecksumCLI(t, args...)
				if prefix == "" && mode == "true" {
					if err == nil || !strings.Contains(stderr, "broken-link") {
						t.Fatalf("following a discovered broken link must fail: err=%v stdout=%q stderr=%s", err, stdout, stderr)
					}
					return
				}
				if err != nil || stdout != "" {
					t.Fatalf("missing prefix or skipped child: err=%v stdout=%q stderr=%s", err, stdout, stderr)
				}
			})
		}
	}
}

func TestFollowLinksCLIInvalidExplicitRoot(t *testing.T) {
	parentDir := t.TempDir()
	followLinksCLISymlink(t, "missing", filepath.Join(parentDir, "broken-link"))
	followLinksCLISymlink(t, "cyclic-link", filepath.Join(parentDir, "cyclic-link"))
	for _, mode := range []string{"default", "false", "true"} {
		for _, root := range []string{"broken-link", "cyclic-link"} {
			t.Run(mode+"/"+root, func(t *testing.T) {
				args := []string{"-j", "1", "-dbfile", filepath.Join(t.TempDir(), "checksums.db"), "-update"}
				if mode != "default" {
					args = append(args, "-followlinks="+mode)
				}
				stdout, stderr, err := runChecksumCLI(t, append(args, filepath.Join(parentDir, root))...)
				if err == nil || !strings.Contains(stderr, root) {
					t.Fatalf("invalid explicit root must fail: err=%v stdout=%q stderr=%s", err, stdout, stderr)
				}
			})
		}
	}
}
