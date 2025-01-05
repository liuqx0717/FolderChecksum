package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func fileCheckWorkerRunTests(t *testing.T, cfg *config,
	mIn []fileCheckMsg, expectOut []dbUpdateMsg) {
	var wg sync.WaitGroup
	tx := make(chan fileCheckMsg)
	rx := make(chan dbUpdateMsg)
	wg.Add(1)
	go fileCheckWorker(cfg, &wg, tx, rx)

	// Send len(mIn) messages.
	go func() {
		for i := range mIn {
			tx <- mIn[i]
		}
		close(tx)
	}()

	// Receive len(expectout) messages.
	for i, expect := range expectOut {
		actual := <-rx
		if actual != expect {
			t.Errorf("actualOut[%d]: %+v", i, actual)
			t.Errorf("expectOut[%d]: %+v", i, expect)
			t.FailNow()
		}
	}
	close(rx)

	wg.Wait()
}

func TestFileCheckWorker(t *testing.T) {
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
		db:       db,
		sizeOnly: false,
		update:   false,
		rootDir:  rootDir,
	}

	// Unchanged files.
	cfg := defaultCfg
	rows := []fileRow{
		{
			path:     "file1",
			size:     5,
			checksum: "826e8142e6baabe8af779f5f490cf5f5",
			visited:  false,
		},
		{
			path: "dir1/file1",
			size: 10,
			// Missing checksum.
			checksum: "",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expect := []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"M", fileInfo{"dir1/file1", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)
	cfg.update = true
	expect = []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"U", fileInfo{"dir1/file1", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)

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
	expect = []dbUpdateMsg{
		{"I", fileInfo{"file1", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"I", fileInfo{"dir1/file1", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	// When cfg.update is false, expect empty dbUpdateMsg.
	fileCheckWorkerRunTests(t, &cfg, mIn, []dbUpdateMsg{})
	cfg.update = true
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)

	// Changed files.
	cfg = defaultCfg
	rows = []fileRow{
		{
			path:     "file1",
			size:     123,
			checksum: "826e8142e6baabe8af779f5f490cf5f5",
			visited:  false,
		},
		{
			path: "dir1/file1",
			size: 10,
			// Missing checksum.
			checksum: "",
			visited:  false,
		},
	}
	clearAndInsertRowsToFiles(t, db, rows)
	expect = []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)
	cfg.update = true
	expect = []dbUpdateMsg{
		{"U", fileInfo{"file1", 5, "826e8142e6baabe8af779f5f490cf5f5"}},
		{"U", fileInfo{"dir1/file1", 10, "a09ebcef8ab11daef0e33e4394ea775f"}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)

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
		db:       db,
		sizeOnly: true,
		update:   false,
		rootDir:  rootDir,
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
	expect := []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, ""}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)
	cfg.update = true
	expect = []dbUpdateMsg{
		{"U", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, ""}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)

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
	expect = []dbUpdateMsg{
		{"I", fileInfo{"file1", 5, ""}},
		{"I", fileInfo{"dir1/file1", 10, ""}},
	}
	// When cfg.update is false, expect empty dbUpdateMsg.
	fileCheckWorkerRunTests(t, &cfg, mIn, []dbUpdateMsg{})
	cfg.update = true
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)

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
	expect = []dbUpdateMsg{
		{"M", fileInfo{"file1", 5, ""}},
		{"M", fileInfo{"dir1/file1", 10, ""}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)
	cfg.update = true
	expect = []dbUpdateMsg{
		{"U", fileInfo{"file1", 5, ""}},
		{"U", fileInfo{"dir1/file1", 10, ""}},
	}
	fileCheckWorkerRunTests(t, &cfg, mIn, expect)
}
