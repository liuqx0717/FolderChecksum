package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const checksumCLIHelperEnv = "FOLDERCHECKSUM_TEST_CLI_HELPER"

// Run the real entry point in a subprocess so fatal errors, flag parsing, and
// concurrent workers behave exactly as they do in the command-line program.
func TestChecksumCLIHelperProcess(t *testing.T) {
	if os.Getenv(checksumCLIHelperEnv) != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing CLI argument separator")
}

func runChecksumCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, append([]string{"-test.run=^TestChecksumCLIHelperProcess$", "--"}, args...)...)
	command.Env = append(os.Environ(), checksumCLIHelperEnv+"=1")
	var out, logs bytes.Buffer
	command.Stdout = &out
	command.Stderr = &logs
	err = command.Run()
	return out.String(), logs.String(), err
}

func checksumCLISeed(t *testing.T, savedChecksum string) (rootDir, dbFile string) {
	t.Helper()
	rootDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "sample.txt"), []byte("sample contents\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dbFile = filepath.Join(t.TempDir(), "checksums.db")
	db := mustOpenDb(dbFile)
	mustCreateFilesTableIfNeeded(db)
	clearAndInsertRowsToFiles(t, db, []fileRow{{
		path: "sample.txt", size: int64(len("sample contents\n")), checksum: savedChecksum,
	}})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return rootDir, dbFile
}

func checksumCLIStored(t *testing.T, dbFile string) string {
	t.Helper()
	db := mustOpenDb(dbFile)
	defer db.Close()
	var checksum string
	if err := db.QueryRow("SELECT COALESCE(checksum, '') FROM files WHERE path = 'sample.txt'").Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	return checksum
}

func TestChecksumCLIAutoWithoutSavedChecksums(t *testing.T) {
	for _, disableAcceleration := range []bool{false, true} {
		t.Run(fmt.Sprintf("disable_acceleration=%t", disableAcceleration), func(t *testing.T) {
			debug := ""
			if disableAcceleration {
				debug = "cpu.all=off"
			}
			t.Setenv("GODEBUG", debug)
			algorithm := "md5"
			want := fmt.Sprintf("%x", md5.Sum([]byte("sample contents\n")))
			if !disableAcceleration && hasSHA1Acceleration() {
				algorithm = "sha1"
				want = fmt.Sprintf("%x", sha1.Sum([]byte("sample contents\n")))
			}
			for _, initialState := range []string{"new_database", "empty_checksum", "null_checksum"} {
				t.Run(initialState, func(t *testing.T) {
					rootDir, dbFile := checksumCLISeed(t, "")
					wantOutput := "changed: sample.txt\n"
					switch initialState {
					case "new_database":
						dbFile = filepath.Join(t.TempDir(), "new.db")
						wantOutput = "new: sample.txt\n"
					case "null_checksum":
						db := mustOpenDb(dbFile)
						_, err := db.Exec("UPDATE files SET checksum = NULL")
						db.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
					stdout, stderr, err := runChecksumCLI(t, "-j", "1", "-dbfile", dbFile, "-update", rootDir)
					if err != nil || stdout != wantOutput {
						t.Fatalf("auto initialization: err=%v stdout=%q stderr=%s", err, stdout, stderr)
					}
					if saved := checksumCLIStored(t, dbFile); saved != want {
						t.Errorf("auto initialization stored %q, want %s %q", saved, algorithm, want)
					}
					log := strings.ToLower(stderr)
					for _, text := range []string{algorithm, "auto", "no saved checksums", runtime.GOOS + "/" + runtime.GOARCH} {
						if !strings.Contains(log, text) {
							t.Errorf("auto initialization log is missing %q: %s", text, stderr)
						}
					}
					if algorithm == "md5" && !strings.Contains(log, "fallback") {
						t.Errorf("auto initialization log must explain the MD5 fallback: %s", stderr)
					}
				})
			}
		})
	}
}

func TestChecksumCLIAutoPreservesSavedAlgorithm(t *testing.T) {
	for _, algorithm := range []struct {
		name   string
		digest string
	}{
		{"md5", fmt.Sprintf("%x", md5.Sum([]byte("sample contents\n")))},
		{"sha1", fmt.Sprintf("%x", sha1.Sum([]byte("sample contents\n")))},
	} {
		for _, explicitAuto := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/explicit_auto=%t", algorithm.name, explicitAuto), func(t *testing.T) {
				rootDir, dbFile := checksumCLISeed(t, algorithm.digest)
				args := []string{"-j", "1", "-dbfile", dbFile}
				if explicitAuto {
					args = append(args, "-checksum", "auto")
				}
				stdout, stderr, err := runChecksumCLI(t, append(args, rootDir)...)
				if err != nil {
					t.Fatalf("CLI failed: %v\n%s", err, stderr)
				}
				if stdout != "" {
					t.Errorf("unchanged content produced output: %q", stdout)
				}
				if saved := checksumCLIStored(t, dbFile); saved != algorithm.digest {
					t.Errorf("comparison changed stored checksum to %q", saved)
				}
				log := strings.ToLower(stderr)
				if !strings.Contains(log, algorithm.name) || !strings.Contains(log, "auto") || !strings.Contains(log, fmt.Sprint(len(algorithm.digest))) {
					t.Errorf("auto mode must log the algorithm and saved checksum length: %s", stderr)
				}
			})
		}
	}
}

func TestChecksumCLIAutoWithUnknownSavedChecksum(t *testing.T) {
	for _, disableAcceleration := range []bool{false, true} {
		t.Run(fmt.Sprintf("disable_acceleration=%t", disableAcceleration), func(t *testing.T) {
			debug := ""
			if disableAcceleration {
				debug = "cpu.all=off"
			}
			t.Setenv("GODEBUG", debug)
			accelerated, platformReason := sha1Acceleration()
			algorithm := "md5"
			wantDigest := fmt.Sprintf("%x", md5.Sum([]byte("sample contents\n")))
			if accelerated {
				algorithm = "sha1"
				wantDigest = fmt.Sprintf("%x", sha1.Sum([]byte("sample contents\n")))
			}
			rootDir, dbFile := checksumCLISeed(t, "abc")
			for _, update := range []bool{false, true} {
				args := []string{"-j", "1", "-dbfile", dbFile}
				if update {
					args = append(args, "-update")
				}
				stdout, stderr, err := runChecksumCLI(t, append(args, rootDir)...)
				if err != nil || stdout != "changed: sample.txt\n" {
					t.Fatalf("unknown saved checksum with update=%t: err=%v stdout=%q stderr=%s", update, err, stdout, stderr)
				}
				want := "abc"
				if update {
					want = wantDigest
				}
				if saved := checksumCLIStored(t, dbFile); saved != want {
					t.Errorf("stored checksum with update=%t: got %q, want %q", update, saved, want)
				}
				for _, text := range []string{
					"Using checksum algorithm: " + algorithm + " (auto:",
					"first nonempty checksum in dbfile has unrecognized length 3",
					platformReason,
				} {
					if !strings.Contains(stderr, text) {
						t.Errorf("auto log with update=%t is missing %q: %s", update, text, stderr)
					}
				}
			}
		})
	}
}

func TestChecksumCLIExplicitAlgorithmChange(t *testing.T) {
	md5Digest := fmt.Sprintf("%x", md5.Sum([]byte("sample contents\n")))
	sha1Digest := fmt.Sprintf("%x", sha1.Sum([]byte("sample contents\n")))
	for _, tc := range []struct {
		name, saved, current string
	}{
		{"sha1", md5Digest, sha1Digest},
		{"md5", sha1Digest, md5Digest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootDir, dbFile := checksumCLISeed(t, tc.saved)
			for _, update := range []bool{false, true} {
				args := []string{"-j", "1", "-dbfile", dbFile, "-checksum", tc.name}
				if update {
					args = append(args, "-update")
				}
				stdout, stderr, err := runChecksumCLI(t, append(args, rootDir)...)
				if err != nil {
					t.Fatalf("CLI failed with update=%t: %v\n%s", update, err, stderr)
				}
				if stdout != "changed: sample.txt\n" {
					t.Errorf("different algorithms with update=%t: got %q", update, stdout)
				}
				want := tc.saved
				if update {
					want = tc.current
				}
				if saved := checksumCLIStored(t, dbFile); saved != want {
					t.Errorf("stored checksum with update=%t: got %q, want %q", update, saved, want)
				}
			}
			stdout, stderr, err := runChecksumCLI(t, "-j", "1", "-dbfile", dbFile, rootDir)
			if err != nil || stdout != "" {
				t.Errorf("auto comparison after algorithm migration: err=%v stdout=%q stderr=%s", err, stdout, stderr)
			}
		})
	}
}

func TestChecksumCLIAutoUsesFirstSavedChecksum(t *testing.T) {
	contents := []byte("sample contents\n")
	md5Digest := fmt.Sprintf("%x", md5.Sum(contents))
	sha1Digest := fmt.Sprintf("%x", sha1.Sum(contents))
	for _, tc := range []struct {
		name, first, second string
	}{
		{"md5", md5Digest, sha1Digest},
		{"sha1", sha1Digest, md5Digest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootDir, dbFile := checksumCLISeed(t, tc.first)
			if err := os.WriteFile(filepath.Join(rootDir, "second.txt"), contents, 0600); err != nil {
				t.Fatal(err)
			}
			db := mustOpenDb(dbFile)
			_, err := db.Exec("INSERT INTO files VALUES ('second.txt', ?, ?, 0)", len(contents), tc.second)
			db.Close()
			if err != nil {
				t.Fatal(err)
			}
			stdout, stderr, err := runChecksumCLI(t, "-j", "1", "-dbfile", dbFile, rootDir)
			if err != nil || stdout != "changed: second.txt\n" {
				t.Fatalf("mixed checksums: err=%v stdout=%q stderr=%s", err, stdout, stderr)
			}
			if !strings.Contains(stderr, "Using checksum algorithm: "+tc.name+" (auto:") {
				t.Errorf("auto mode did not log the first checksum's algorithm: %s", stderr)
			}
		})
	}
}

func TestChecksumCLIInvalidOption(t *testing.T) {
	rootDir := t.TempDir()
	dbFile := filepath.Join(t.TempDir(), "must-not-create.db")
	stdout, stderr, err := runChecksumCLI(t, "-checksum", "sha256", "-dbfile", dbFile, rootDir)
	if err == nil {
		t.Fatal("unsupported checksum algorithm succeeded")
	}
	if stdout != "" || !strings.Contains(strings.ToLower(stderr), "checksum") || !strings.Contains(stderr, "sha256") {
		t.Errorf("unexpected invalid algorithm output: stdout=%q stderr=%s", stdout, stderr)
	}
	if _, err := os.Stat(dbFile); !os.IsNotExist(err) {
		t.Errorf("invalid algorithm must fail before opening the database: %v", err)
	}
}

func TestChecksumCLIHelp(t *testing.T) {
	stdout, stderr, err := runChecksumCLI(t, "-help")
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, stderr)
	}
	help := strings.ToLower(stdout + stderr)
	for _, text := range []string{"-checksum", "auto", "sha1", "md5", "mismatch", "changed"} {
		if !strings.Contains(help, text) {
			t.Errorf("help is missing %q", text)
		}
	}
}

func TestChecksumCLISizeOnly(t *testing.T) {
	for _, algorithm := range []string{"auto", "sha1", "md5"} {
		t.Run(algorithm, func(t *testing.T) {
			// An unrecognized stored checksum must not affect size-only checks.
			rootDir, dbFile := checksumCLISeed(t, "old-checksum")
			for _, update := range []bool{false, true} {
				args := []string{"-j", "1", "-dbfile", dbFile, "-sizeonly", "-checksum", algorithm}
				if update {
					args = append(args, "-update")
				}
				stdout, stderr, err := runChecksumCLI(t, append(args, rootDir)...)
				if err != nil || stdout != "" {
					t.Fatalf("size-only check with update=%t: err=%v stdout=%q stderr=%s", update, err, stdout, stderr)
				}
				want := "old-checksum"
				if update {
					want = ""
				}
				if saved := checksumCLIStored(t, dbFile); saved != want {
					t.Errorf("stored size-only checksum with update=%t: got %q, want %q", update, saved, want)
				}
			}
		})
	}
}
