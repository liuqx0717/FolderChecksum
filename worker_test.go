package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func fileCheckWorkerRunTests(t *testing.T, cfg *config,
	mIn []fileCheckMsg, expectMOut []dbUpdateMsg, expectStdout string) {
	var wg sync.WaitGroup
	var builder strings.Builder
	tx := make(chan fileCheckMsg)
	rx := make(chan dbUpdateMsg)

	origOutFile := cfg.outFile
	defer func() { cfg.outFile = origOutFile }()
	cfg.outFile = &builder

	wg.Add(1)
	go fileCheckWorker(0, cfg, &wg, tx, rx)

	// Send len(mIn) messages.
	go func() {
		for _, m := range mIn {
			tx <- m
		}
		close(tx)
	}()

	// Receive len(expectout) messages.
	for i, expect := range expectMOut {
		actual := <-rx
		if actual != expect {
			t.Errorf("actualOut[%d]: %+v", i, actual)
			t.Errorf("expectOut[%d]: %+v", i, expect)
			t.FailNow()
		}
	}
	close(rx)
	wg.Wait()

	actualStdOut := builder.String()
	if actualStdOut != expectStdout {
		t.Errorf("actualStdout: %s", actualStdOut)
		t.Errorf("expectStdout: %s", expectStdout)
		t.FailNow()
	}
}

func TestFileCheckWorker(t *testing.T) {
	// - rootDir
	// | file1exc
	// | file.exc
	// | exclude
	// | - dir1.exc
	// | | incfile1.exc
	rootDir := filepath.Join(t.TempDir(), "rootDir")
	err := os.Mkdir(rootDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(rootDir, "file1exc"), []byte("file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(rootDir, "file.exc"), []byte("file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(rootDir, "exclude"), []byte("file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	dir1 := filepath.Join(rootDir, "dir1.exc")
	err = os.Mkdir(dir1, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir1, "incfile1.exc"), []byte("dir1/file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	mIn := []fileCheckMsg{
		{"exclude", 5},
		{"file1exc", 5},
		{"file.exc", 5},
		{"dir1.exc/incfile1.exc", 10},
	}

	db := prepareTestDb(t)
	defer db.Close()

	defaultCfg := config{
		db:        db,
		excludeRe: regexp.MustCompile(`^((.*\.exc)|(exclude))$`),
		includeRe: regexp.MustCompile(`^(.*/)?inc[^/]*$`),
		sizeOnly:  false,
		update:    false,
		rootDir:   rootDir,
	}

	// Unchanged files.
	cfg := defaultCfg
	rows := []fileRow{
		{
			path:     "file1exc",
			size:     5,
			checksum: "826e8142e6baabe8af779f5f490cf5f5",
			visited:  false,
		},
		{
			path: "dir1.exc/incfile1.exc",
			size: 10,
			// Missing checksum.
			checksum: "",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expectMOut := []dbUpdateMsg{
		{"M", fileInfo{"file1exc", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"M", fileInfo{"dir1.exc/incfile1.exc", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	expectStdout := "changed: dir1.exc/incfile1.exc\n"
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)
	cfg.update = true
	expectMOut = []dbUpdateMsg{
		{"M", fileInfo{"file1exc", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"U", fileInfo{"dir1.exc/incfile1.exc", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)

	// New files.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path:     "dir1.exc",
			size:     10,
			checksum: "a09ebcef8ab11daef0e33e4394ea775f",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expectMOut = []dbUpdateMsg{
		{"I", fileInfo{"file1exc", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"I", fileInfo{"dir1.exc/incfile1.exc", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	expectStdout = "new: file1exc\n" +
		"new: dir1.exc/incfile1.exc\n"
	// When cfg.update is false, expect empty dbUpdateMsg.
	fileCheckWorkerRunTests(t, &cfg, mIn, []dbUpdateMsg{}, expectStdout)
	cfg.update = true
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)

	// Changed files.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path:     "file1exc",
			size:     123,
			checksum: "826e8142e6baabe8af779f5f490cf5f5",
			visited:  false,
		},
		{
			path: "dir1.exc/incfile1.exc",
			size: 10,
			// Missing checksum.
			checksum: "",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expectMOut = []dbUpdateMsg{
		{"M", fileInfo{"file1exc", 5, ""}},
		{"M", fileInfo{"dir1.exc/incfile1.exc", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	expectStdout = "changed: file1exc\n" +
		"changed: dir1.exc/incfile1.exc\n"
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)
	cfg.update = true
	expectMOut = []dbUpdateMsg{
		{"U", fileInfo{"file1exc", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"U", fileInfo{"dir1.exc/incfile1.exc", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)
}

func TestFileCheckWorkerSizeOnly(t *testing.T) {
	// - rootDir
	// | file1
	// | - dir1
	// | | file1
	rootDir := filepath.Join(t.TempDir(), "rootDir")
	err := os.Mkdir(rootDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(rootDir, "file1"), []byte("file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	dir1 := filepath.Join(rootDir, "dir1")
	err = os.Mkdir(dir1, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir1, "file1"), []byte("dir1/file1"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	mIn := []fileCheckMsg{
		{"file1", 5},
		{"dir1/file1", 10},
	}

	db := prepareTestDb(t)
	defer db.Close()

	defaultCfg := config{
		db:        db,
		excludeRe: regexp.MustCompile(`^$`),
		includeRe: regexp.MustCompile(`^$`),
		sizeOnly:  true,
		update:    false,
		rootDir:   rootDir,
	}

	// Unchanged files.
	cfg := defaultCfg
	rows := []fileRow{
		{
			path: "file1",
			size: 5,
			// Redundant checksum.
			checksum: "abcde",
			visited:  false,
		},
		{
			path:     "dir1/file1",
			size:     10,
			checksum: "",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expectMOut := []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, ""}},
	}
	expectStdout := ""
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)
	cfg.update = true
	expectMOut = []dbUpdateMsg{
		{"U", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, ""}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)

	// New files.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path:     "dir1",
			size:     10,
			checksum: "a09ebcef8ab11daef0e33e4394ea775f",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expectMOut = []dbUpdateMsg{
		{"I", fileInfo{"file1", 5, ""}},
		{"I", fileInfo{"dir1/file1", 10, ""}},
	}
	expectStdout = "new: file1\n" +
		"new: dir1/file1\n"
	// When cfg.update is false, expect empty dbUpdateMsg.
	fileCheckWorkerRunTests(t, &cfg, mIn, []dbUpdateMsg{}, expectStdout)
	cfg.update = true
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)

	// Changed files.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path: "file1",
			size: 123,
			// Redundant checksum.
			checksum: "826e8142e6baabe8af779f5f490cf5f5",
			visited:  false,
		},
		{
			path:     "dir1/file1",
			size:     123,
			checksum: "",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expectMOut = []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, ""}},
	}
	expectStdout = "changed: file1\n" +
		"changed: dir1/file1\n"
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)
	cfg.update = true
	expectMOut = []dbUpdateMsg{
		{"U", fileInfo{"file1", 5, ""}},
		{"U", fileInfo{"dir1/file1", 10, ""}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expectMOut, expectStdout)
}

func dbUpdateWorkerRunTest(t *testing.T, cfg *config,
	mIn []dbUpdateMsg, expectRows []fileRow, expectStdout string) {
	var wg sync.WaitGroup
	var builder strings.Builder
	tx := make(chan dbUpdateMsg)

	origOutFile := cfg.outFile
	defer func() { cfg.outFile = origOutFile }()
	cfg.outFile = &builder

	clearStats()
	wg.Add(1)
	go dbUpdateWorker(cfg, &wg, tx)

	for _, m := range mIn {
		// In this test we don't create fileCheckWorker, so update the
		// stats here.
		switch m.opType {
		case "I":
			stats.numFilesNew.Add(1)
		case "U":
			stats.numFilesChanged.Add(1)
		case "M":
			stats.numFilesUnchanged.Add(1)
		case "D":
		default:
			t.Fatalf("Unknown opType %s", m.opType)
		}
		tx <- m
	}
	close(tx)
	wg.Wait()

	actualRows := getAllRowsFromFiles(t, cfg.db)
	verifyFileRows(t, actualRows, expectRows)

	actualStdOut := builder.String()
	if actualStdOut != expectStdout {
		t.Errorf("actualStdout: %s", actualStdOut)
		t.Errorf("expectStdout: %s", expectStdout)
		t.FailNow()
	}
}

func TestDbUpdateWorker(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	defaultCfg := config{
		db:     db,
		update: false,
	}

	// Insert, update, mark.
	cfg := defaultCfg
	rows := []fileRow{
		{
			path:     "file1",
			size:     5,
			checksum: "aaa",
			visited:  false,
		},
		{
			path:     "dir1/file1",
			size:     10,
			checksum: "bbb",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	mIn := []dbUpdateMsg{
		{"I", fileInfo{"dir1/file2", 20, "ccc"}},
		{"U", fileInfo{"dir1/file1", 20, "ccc"}},
		{"M", fileInfo{"file1", 0, ""}},
		{"D", fileInfo{"", 0, ""}},
	}
	expectRows := []fileRow{
		{
			path:     "dir1/file1",
			size:     20,
			checksum: "ccc",
			visited:  false,
		},
		{
			path:     "dir1/file2",
			size:     20,
			checksum: "ccc",
			visited:  false,
		},
		{
			path:     "file1",
			size:     5,
			checksum: "aaa",
			visited:  false,
		},
	}
	expectStdout := ""
	dbUpdateWorkerRunTest(t, &cfg, mIn, copyAndSortFileRows(rows), expectStdout)
	cfg.update = true
	dbUpdateWorkerRunTest(t, &cfg, mIn, expectRows, expectStdout)

	// Originally dir1 was a folder in db. Then it becomes a file.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path:     "file1",
			size:     5,
			checksum: "aaa",
			visited:  false,
		},
		{
			path:     "dir1/file1",
			size:     10,
			checksum: "bbb",
			visited:  false,
		},
		{
			path:     "dir1/file2",
			size:     10,
			checksum: "ccc",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	mIn = []dbUpdateMsg{
		{"I", fileInfo{"dir1", 20, "ddd"}},
		{"D", fileInfo{"dir1", 0, ""}},
	}
	expectRows = []fileRow{
		{
			path:     "dir1",
			size:     20,
			checksum: "ddd",
			visited:  false,
		},
		{
			path:     "file1",
			size:     5,
			checksum: "aaa",
			visited:  false,
		},
	}
	expectStdout = "deleted: dir1/file1\n" +
		"deleted: dir1/file2\n"
	dbUpdateWorkerRunTest(t, &cfg, mIn, copyAndSortFileRows(rows), expectStdout)
	cfg.update = true
	dbUpdateWorkerRunTest(t, &cfg, mIn, expectRows, expectStdout)

	// Originally dir1 was a file in db. Then it becomes a folder.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path:     "file1",
			size:     5,
			checksum: "aaa",
			visited:  false,
		},
		{
			path:     "dir1",
			size:     5,
			checksum: "bbb",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	mIn = []dbUpdateMsg{
		{"I", fileInfo{"dir1/file1", 10, "ccc"}},
		{"I", fileInfo{"dir1/file2", 10, "ddd"}},
		{"D", fileInfo{"dir1", 0, ""}},
	}
	expectRows = []fileRow{
		{
			path:     "dir1/file1",
			size:     10,
			checksum: "ccc",
			visited:  false,
		},
		{
			path:     "dir1/file2",
			size:     10,
			checksum: "ddd",
			visited:  false,
		},
		{
			path:     "file1",
			size:     5,
			checksum: "aaa",
			visited:  false,
		},
	}
	expectStdout = "deleted: dir1\n"
	dbUpdateWorkerRunTest(t, &cfg, mIn, copyAndSortFileRows(rows), expectStdout)
	cfg.update = true
	dbUpdateWorkerRunTest(t, &cfg, mIn, expectRows, expectStdout)
}
