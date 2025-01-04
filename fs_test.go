package main

import (
	"os"
	"path/filepath"
	"testing"
)

type walkRes struct {
	path string
	size int64
}

func verifyWalkRes(t *testing.T, actual []walkRes, expect []walkRes) {
	if len(actual) != len(expect) {
		t.Errorf("actual: %+v", actual)
		t.Errorf("expect: %+v", expect)
		t.FailNow()
	}
	for i := range actual {
		if actual[i] != expect[i] {
			t.Errorf("actual[%d]: %+v", i, actual[i])
			t.Errorf("expect[%d]: %+v", i, expect[i])
		}
	}
}

// - testDir
// | file1
// | file2
// | emptyFile
// | - dir1
// | | file1
// | | file2
// | | - emptyDir
// | - dir2
// | | file1 -> ../file1
// | | dir1 -> ../dir1
func prepareTestDir(t *testing.T) string {
	testDir := filepath.Join(t.TempDir(), "testDir")
	err := os.Mkdir(testDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(testDir, "file1"), []byte("file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(testDir, "file2"), []byte("file2"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(testDir, "emptyFile"), []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dir1 := filepath.Join(testDir, "dir1")
	err = os.Mkdir(dir1, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir(filepath.Join(dir1, "emptyDir"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir1, "file1"), []byte("dir1/file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir1, "file2"), []byte("dir1/file2"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dir2 := filepath.Join(testDir, "dir2")
	err = os.Mkdir(dir2, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(filepath.Join("..", "file1"), filepath.Join(dir2, "file1"))
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(filepath.Join("..", "dir1"), filepath.Join(dir2, "dir1"))
	if err != nil {
		t.Fatal(err)
	}

	return testDir
}

func TestCalcFileMd5(t *testing.T) {
	// % echo -n '' | md5sum
	// d41d8cd98f00b204e9800998ecf8427e  -
	// % echo -n 'file1' | md5sum
	// 826e8142e6baabe8af779f5f490cf5f5  -
	// % echo -n 'dir1/file1' | md5sum
	// a09ebcef8ab11daef0e33e4394ea775f  -

	rootDir := prepareTestDir(t)

	// Empty file.
	md5, n := mustCalcFileMd5(filepath.Join(rootDir, "emptyFile"))
	if md5 != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatalf("Incorrect md5: %s", md5)
	}
	if n != 0 {
		t.Fatalf("Incorrect n: %d", n)
	}

	// Non-empty file.
	md5, n = mustCalcFileMd5(filepath.Join(rootDir, "file1"))
	if md5 != "826e8142e6baabe8af779f5f490cf5f5" {
		t.Fatalf("Incorrect md5: %s", md5)
	}
	if n != 5 {
		t.Fatalf("Incorrect n: %d", n)
	}

	// Symlink to a file.
	md5, n = mustCalcFileMd5(filepath.Join(rootDir, "dir2", "file1"))
	if md5 != "826e8142e6baabe8af779f5f490cf5f5" {
		t.Fatalf("Incorrect md5: %s", md5)
	}
	if n != 5 {
		t.Fatalf("Incorrect n: %d", n)
	}

	// Symlink in path.
	md5, n = mustCalcFileMd5(filepath.Join(rootDir, "dir2", "dir1", "file1"))
	if md5 != "a09ebcef8ab11daef0e33e4394ea775f" {
		t.Fatalf("Incorrect md5: %s", md5)
	}
	if n != 10 {
		t.Fatalf("Incorrect n: %d", n)
	}
}

func TestWalkDirIgnoreSymLinks(t *testing.T) {
	var actual []walkRes
	var expect []walkRes
	procOneFile := func(relPath string, size int64) {
		actual = append(actual, walkRes{relPath, size})
	}

	rootDir := prepareTestDir(t)

	// Walk the whole rootDir.
	expect = []walkRes{
		{"dir1/file1", 10},
		{"dir1/file2", 10},
		{"emptyFile", 0},
		{"file1", 5},
		{"file2", 5},
	}
	actual = []walkRes{}
	mustWalkDir(rootDir, "", false, procOneFile)
	verifyWalkRes(t, actual, expect)
	actual = []walkRes{}
	mustWalkDir(rootDir, "../../", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use the subdir dir1 as prefix.
	expect = []walkRes{
		{"dir1/file1", 10},
		{"dir1/file2", 10},
	}
	actual = []walkRes{}
	mustWalkDir(rootDir, "dir1/", false, procOneFile)
	verifyWalkRes(t, actual, expect)
	actual = []walkRes{}
	mustWalkDir(rootDir, "dir1/../../../dir1", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use the subdir dir2, dir1/emptyDir as prefix.
	actual = []walkRes{}
	expect = []walkRes{}
	mustWalkDir(rootDir, "dir2", false, procOneFile)
	verifyWalkRes(t, actual, expect)
	mustWalkDir(rootDir, "dir1/emptyDir/", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use the subfile file1 as prefix.
	actual = []walkRes{}
	expect = []walkRes{
		{"file1", 5},
	}
	mustWalkDir(rootDir, "file1", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use a non-existing subdir as prefix.
	actual = []walkRes{}
	expect = []walkRes{}
	mustWalkDir(rootDir, "dirX", false, procOneFile)
	verifyWalkRes(t, actual, expect)
	mustWalkDir(rootDir, "dir1/dirX", false, procOneFile)
	verifyWalkRes(t, actual, expect)
	mustWalkDir(rootDir, "dirX/dirX", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use the symlink file1 as prefix. It's followed.
	actual = []walkRes{}
	expect = []walkRes{
		{"dir2/file1", 5},
	}
	mustWalkDir(rootDir, "dir2/file1", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use the symlink dir1 as prefix. It's followed.
	actual = []walkRes{}
	expect = []walkRes{
		{"dir2/dir1/file1", 10},
		{"dir2/dir1/file2", 10},
	}
	mustWalkDir(rootDir, "dir2/dir1", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use a prefix that has a symlink in between. It's followed.
	actual = []walkRes{}
	expect = []walkRes{
		{"dir2/dir1/file1", 10},
	}
	mustWalkDir(rootDir, "dir2/dir1/file1", false, procOneFile)
	verifyWalkRes(t, actual, expect)

	// Use the symlink dir1 as rootDir. It's followed.
	actual = []walkRes{}
	expect = []walkRes{
		{"file1", 10},
		{"file2", 10},
	}
	mustWalkDir(filepath.Join(rootDir, "dir2", "dir1"), "", false, procOneFile)
	verifyWalkRes(t, actual, expect)
}
