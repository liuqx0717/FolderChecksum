package main

import (
	"database/sql"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type fileRow struct {
	path     string
	size     int64
	checksum any // string or nil
	visited  bool
}

var testDbRows = [...]fileRow{
	{
		path:     "file1",
		size:     123,
		checksum: "aaa",
		visited:  true,
	},
	{
		path:     "file2",
		size:     123,
		checksum: "bbb",
		visited:  false,
	},
	{
		// This means originally file2 was a file when creating the db,
		// later when scanning the folder again, we find a new folder
		// with the same name "file2".
		path:     "file2/file1",
		size:     123,
		checksum: "bbb",
		visited:  true,
	},
	{
		// The name starts with special character '%' (sql wildcard)
		path:     "%dir1/file1",
		size:     456,
		checksum: nil,
		visited:  false,
	},
	{
		// The name starts with special character '%'
		path:     "%dir1/file2",
		size:     456,
		checksum: nil,
		visited:  true,
	},
	{
		// The name starts with special character '%'
		path:     "%dir1/dir1/file1",
		size:     789,
		checksum: "ccc",
		visited:  true,
	},
	{
		// The name starts with special character '%'
		path:     "%dir1/dir1/file2",
		size:     789,
		checksum: "ddd",
		visited:  false,
	},
	{
		// The name starts with special character '%', it also has
		// prefix %dir1 but it's a different folder.
		path:     "%dir123/file1",
		size:     789,
		checksum: "eee",
		visited:  false,
	},
	{
		// The name contains special characters \ _ " ' `
		path:     "dir\\_2/dir1/\"'`file1",
		size:     math.MaxUint32 * 10,
		checksum: "fff",
		visited:  false,
	},
	{
		// The name contains special characters \ _ " ' `
		path:     "dir\\_2/dir1/\"'`file2",
		size:     math.MaxUint32 * 10,
		checksum: "ggg",
		visited:  true,
	},
}

func copyAndSortFileRows(rows []fileRow) []fileRow {
	ret := make([]fileRow, len(rows))
	copy(ret, rows)
	sort.Slice(ret, func(i int, j int) bool {
		return ret[i].path < ret[j].path
	})
	return ret
}

func verifyFileRows(t *testing.T, actual []fileRow, expect []fileRow) {
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

func verifyFileInfo(t *testing.T, actual []fileInfo, expect []fileInfo) {
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

func getAllRowsFromFiles(t *testing.T, db *sql.DB) []fileRow {
	rows, err := db.Query(
		`SELECT path, size, checksum, visited FROM files
			ORDER BY path ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var ret []fileRow
	for rows.Next() {
		var row fileRow
		err = rows.Scan(&row.path, &row.size, &row.checksum, &row.visited)
		if err != nil {
			t.Fatal(err)
		}
		ret = append(ret, row)
	}

	return ret
}

func clearAndInsertRowsToFiles(t *testing.T, db *sql.DB, rows []fileRow) {
	tx := mustCreateTx(db)
	_, err := tx.Exec("DELETE FROM files")
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(
		`INSERT INTO files(path, size, checksum, visited)
			VALUES(?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		_, err = stmt.Exec(row.path, row.size, row.checksum, row.visited)
		if err != nil {
			t.Fatal(err)
		}
	}
	mustCommitTx(tx)
}

// The user should call Close() on the return value.
func prepareTestDb(t *testing.T) *sql.DB {
	dbFile := filepath.Join(t.TempDir(), "test.db")

	db := mustOpenDb(dbFile)
	mustCreateFilesTableIfNeeded(db)
	mustCreateFilesTableIfNeeded(db)
	return db
}

func TestCreateFilesTable(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()
}

func TestInsertFile(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	tx := mustCreateTx(db)
	stmt := mustPrepareInsertFile(tx)
	defer stmt.Close()

	for _, row := range testDbRows {
		file := fileInfo{
			relPath:  row.path,
			size:     row.size,
			checksum: "",
		}
		if row.checksum != nil {
			file.checksum = row.checksum.(string)
		}
		mustInsertFile(stmt, &file)
	}

	mustCommitTx(tx)

	actualRows := getAllRowsFromFiles(t, db)
	expectRows := copyAndSortFileRows(testDbRows[:])
	for i := range expectRows {
		expectRows[i].visited = true
	}
	verifyFileRows(t, actualRows, expectRows)
}

func TestUpdateAndMarkFile(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	clearAndInsertRowsToFiles(t, db, testDbRows[:])

	tx := mustCreateTx(db)
	stmt := mustPrepareUpdateAndMarkFile(tx)
	defer stmt.Close()

	for _, row := range testDbRows {
		mustUpdateAndMarkFile(stmt, &fileInfo{
			relPath:  row.path,
			size:     math.MaxInt64,
			checksum: "newchecksum",
		})
	}

	mustCommitTx(tx)

	actualRows := getAllRowsFromFiles(t, db)
	expectRows := copyAndSortFileRows(testDbRows[:])
	for i := range expectRows {
		expectRows[i].size = math.MaxInt64
		expectRows[i].checksum = "newchecksum"
		expectRows[i].visited = true
	}
	verifyFileRows(t, actualRows, expectRows)
}

func TestMarkFile(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	clearAndInsertRowsToFiles(t, db, testDbRows[:])

	tx := mustCreateTx(db)
	stmt := mustPrepareMarkFile(tx)
	defer stmt.Close()

	mustMarkFile(stmt, "%dir1/file1")

	mustCommitTx(tx)

	actualRows := getAllRowsFromFiles(t, db)
	expectRows := copyAndSortFileRows(testDbRows[:])
	for i := range expectRows {
		if expectRows[i].path == "%dir1/file1" {
			expectRows[i].visited = true
		}
	}
	verifyFileRows(t, actualRows, expectRows)
}

func TestQueryFile(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	clearAndInsertRowsToFiles(t, db, testDbRows[:])

	// Query a non-existing file
	file, visited := mustQueryFile(db, "fileX")
	if file != nil || visited {
		t.Errorf("nil expected: %+v", file)
	}

	// Query an existing folder name
	file, visited = mustQueryFile(db, "%dir1")
	if file != nil || visited {
		t.Errorf("nil expected: %+v", file)
	}

	// Query existing file names
	for _, row := range testDbRows {
		actual, visited := mustQueryFile(db, row.path)
		expect := fileInfo{
			relPath:  row.path,
			size:     row.size,
			checksum: "",
		}
		if row.checksum != nil {
			expect.checksum = row.checksum.(string)
		}
		if actual != expect {
			t.Errorf("actual: %+v", actual)
			t.Errorf("expect: %+v", expect)
		}
		if visited != row.visited {
			t.Errorf("Incorrect visited flag for %+v", row)
		}
	}
}

func TestDeleteUnvisitedFile(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	var actualRows []fileRow
	var expectRows []fileRow

	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx := mustCreateTx(db)
	mustDeleteUnvisitedFile(tx, "file2")
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = []fileRow{}
	for _, row := range copyAndSortFileRows(testDbRows[:]) {
		if row.path == "file2" {
			continue
		}
		expectRows = append(expectRows, row)
	}
	verifyFileRows(t, actualRows, expectRows)
}

func TestQueryUnvisitedFiles(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	clearAndInsertRowsToFiles(t, db, testDbRows[:])

	var actual []fileInfo
	var expect []fileInfo
	procOneFile := func(file *fileInfo) {
		actual = append(actual, *file)
	}

	tx := mustCreateTx(db)

	// Query all files.
	actual = []fileInfo{}
	expect = []fileInfo{}
	for _, row := range copyAndSortFileRows(testDbRows[:]) {
		if row.visited {
			continue
		}
		file := fileInfo{
			relPath:  row.path,
			size:     row.size,
			checksum: "",
		}
		if row.checksum != nil {
			file.checksum = row.checksum.(string)
		}
		expect = append(expect, file)
	}
	mustQueryUnvisitedFiles(tx, "", procOneFile)
	verifyFileInfo(t, actual, expect)

	// Query subfolder %dir1.
	actual = []fileInfo{}
	expect = []fileInfo{
		{
			relPath:  "%dir1/dir1/file2",
			size:     789,
			checksum: "ddd",
		},
		{
			relPath:  "%dir1/file1",
			size:     456,
			checksum: "",
		},
	}
	mustQueryUnvisitedFiles(tx, "%dir1/", procOneFile)
	verifyFileInfo(t, actual, expect)

	// Query subfolder dir\_2.
	actual = []fileInfo{}
	expect = []fileInfo{
		{
			relPath:  "dir\\_2/dir1/\"'`file1",
			size:     math.MaxUint32 * 10,
			checksum: "fff",
		},
	}
	mustQueryUnvisitedFiles(tx, `dir\_2/`, procOneFile)
	verifyFileInfo(t, actual, expect)

	// Query a non-existing folder.
	actual = []fileInfo{}
	expect = []fileInfo{}
	mustQueryUnvisitedFiles(tx, "dirXXX/", procOneFile)
	verifyFileInfo(t, actual, expect)

	mustCommitTx(tx)
}

func TestDeleteUnvisitedFiles(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	var actualRows []fileRow
	var expectRows []fileRow

	// Delete all unvisited files.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx := mustCreateTx(db)
	mustDeleteUnvisitedFiles(tx, "", 5)
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = []fileRow{}
	for _, row := range copyAndSortFileRows(testDbRows[:]) {
		if !row.visited {
			continue
		}
		expectRows = append(expectRows, row)
	}
	verifyFileRows(t, actualRows, expectRows)

	// Delete all unvisited files in %dir1.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx = mustCreateTx(db)
	mustDeleteUnvisitedFiles(tx, "%dir1/", 2)
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = []fileRow{}
	for _, row := range copyAndSortFileRows(testDbRows[:]) {
		if strings.HasPrefix(row.path, "%dir1/") && !row.visited {
			continue
		}
		expectRows = append(expectRows, row)
	}
	verifyFileRows(t, actualRows, expectRows)

	// Delete all unvisited files in dir\_2.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx = mustCreateTx(db)
	mustDeleteUnvisitedFiles(tx, `dir\_2/`, 1)
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = []fileRow{}
	for _, row := range copyAndSortFileRows(testDbRows[:]) {
		if strings.HasPrefix(row.path, `dir\_2/`) && !row.visited {
			continue
		}
		expectRows = append(expectRows, row)
	}
	verifyFileRows(t, actualRows, expectRows)

	// Delete all unvisited files in a non-existing folder.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx = mustCreateTx(db)
	mustDeleteUnvisitedFiles(tx, `dirXXX/`, 0)
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = copyAndSortFileRows(testDbRows[:])
	verifyFileRows(t, actualRows, expectRows)
}

func TestClearVisitedFlag(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	var actualRows []fileRow
	var expectRows []fileRow

	clearAndInsertRowsToFiles(t, db, testDbRows[:])

	// Clear existing file names
	tx := mustCreateTx(db)
	for _, row := range testDbRows {
		if row.visited {
			mustClearVisitedFlag(tx, row.path)
		}
	}
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = copyAndSortFileRows(testDbRows[:])
	for i := range expectRows {
		expectRows[i].visited = false
	}
	verifyFileRows(t, actualRows, expectRows)
}

func TestClearVisitedFlags(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()

	var actualRows []fileRow
	var expectRows []fileRow

	// Clear all files.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx := mustCreateTx(db)
	n := mustClearVisitedFlags(tx, "")
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = copyAndSortFileRows(testDbRows[:])
	for i := range expectRows {
		expectRows[i].visited = false
	}
	verifyFileRows(t, actualRows, expectRows)
	if n != 5 {
		t.Fatalf("Incorrect n=%d", n)
	}
	_, err := db.Exec("DELETE FROM files")
	if err != nil {
		t.Fatal(err)
	}

	// Clear files in %dir1.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx = mustCreateTx(db)
	n = mustClearVisitedFlags(tx, "%dir1/")
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = copyAndSortFileRows(testDbRows[:])
	for i, row := range expectRows {
		if strings.HasPrefix(row.path, "%dir1/") {
			expectRows[i].visited = false
		}
	}
	verifyFileRows(t, actualRows, expectRows)
	if n != 2 {
		t.Fatalf("Incorrect n=%d", n)
	}
	_, err = db.Exec("DELETE FROM files")
	if err != nil {
		t.Fatal(err)
	}

	// Clear files in dir\_2.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx = mustCreateTx(db)
	n = mustClearVisitedFlags(tx, `dir\_2/`)
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = copyAndSortFileRows(testDbRows[:])
	for i, row := range expectRows {
		if strings.HasPrefix(row.path, `dir\_2/`) {
			expectRows[i].visited = false
		}
	}
	verifyFileRows(t, actualRows, expectRows)
	if n != 1 {
		t.Fatalf("Incorrect n=%d", n)
	}
	_, err = db.Exec("DELETE FROM files")
	if err != nil {
		t.Fatal(err)
	}

	// Clear files in a non-existing folder.
	clearAndInsertRowsToFiles(t, db, testDbRows[:])
	tx = mustCreateTx(db)
	n = mustClearVisitedFlags(tx, `dirXXX/`)
	mustCommitTx(tx)
	actualRows = getAllRowsFromFiles(t, db)
	expectRows = copyAndSortFileRows(testDbRows[:])
	verifyFileRows(t, actualRows, expectRows)
	if n != 0 {
		t.Fatalf("Incorrect n=%d", n)
	}
	_, err = db.Exec("DELETE FROM files")
	if err != nil {
		t.Fatal(err)
	}
}
