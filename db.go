package main

import (
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type fileInfo struct {
	path     string
	size     int64
	checksum string
}

func escapeForLike(literal string) string {
	ret := strings.ReplaceAll(literal, `\`, `\\`)
	ret = strings.ReplaceAll(ret, `%`, `\%`)
	ret = strings.ReplaceAll(ret, `_`, `\_`)
	return ret
}

// The user should call Close() on the return value.
func mustOpenDb(file string) *sql.DB {
	db, err := sql.Open("sqlite3", "file:"+file+
		"?_journal_mode=DELETE&_txlock=immediate")
	if err != nil {
		logFatal("Failed to open '%s': %s", file, err.Error())
	}
	return db
}

// The user should call Commit() or Rollback() on the return value.
func mustCreateTx(db *sql.DB) *sql.Tx {
	tx, err := db.Begin()
	if err != nil {
		logFatal("Failed to create tx: %s", err.Error())
	}
	return tx
}

func mustCommitTx(tx *sql.Tx) {
	err := tx.Commit()
	if err != nil {
		logFatal("Failed to commit tx: %s", err.Error())
	}
}

func mustCreateFilesTable(db *sql.DB) {
	sqlStr :=
		`CREATE TABLE files (
	    	path TEXT NOT NULL PRIMARY KEY,
			size INT NOT NULL,
			checksum TEXT NULL,
			visited BIT NOT NULL)`

	tx := mustCreateTx(db)

	_, err := tx.Exec(sqlStr)
	if err != nil {
		logFatal("Failed to create table: %s", err.Error())
	}

	mustCommitTx(tx)
}

func assertRowsAffected(res sql.Result, n int64) {
	numRows, err := res.RowsAffected()
	if err != nil {
		logFatal("Failed to get rows affected: %s", err.Error())
	}
	if numRows != n {
		logFatal("RowsAffected should be %d, but got %d", n, numRows)
	}
}

// The user should call Commit() or Rollback() on tx, or Close()
// on the return value.
func mustPrepareInsertFile(tx *sql.Tx) *sql.Stmt {
	stmt, err := tx.Prepare(
		`INSERT INTO files(path, size, checksum, visited)
			VALUES(?, ?, ?, 1)`)
	if err != nil {
		logFatal("Failed to prepare insert: %s", err.Error())
	}
	return stmt
}

func mustInsertFile(stmt *sql.Stmt, file *fileInfo) {
	var res sql.Result
	var err error

	if file.checksum == "" {
		res, err = stmt.Exec(file.path, file.size, nil)
	} else {
		res, err = stmt.Exec(file.path, file.size, file.checksum)
	}
	if err != nil {
		logFatal("Failed to insert %+v: %s", file, err.Error())
	}
	assertRowsAffected(res, 1)
}

// The user should call Commit() or Rollback() on tx, or Close()
// on the return value.
func mustPrepareUpdateAndMarkFile(tx *sql.Tx) *sql.Stmt {
	stmt, err := tx.Prepare(
		`UPDATE files
			SET size=?, checksum=?, visited=1
			WHERE path=?`)
	if err != nil {
		logFatal("Failed to prepare update: %s", err.Error())
	}
	return stmt
}

func mustUpdateAndMarkFile(stmt *sql.Stmt, file *fileInfo) {
	var res sql.Result
	var err error

	if file.checksum == "" {
		res, err = stmt.Exec(file.size, nil, file.path)
	} else {
		res, err = stmt.Exec(file.size, file.checksum, file.path)
	}
	if err != nil {
		logFatal("Failed to update %+v: %s", file, err.Error())
	}
	assertRowsAffected(res, 1)
}

// The user should call Commit() or Rollback() on tx, or Close()
// on the return value.
func mustPrepareMarkFile(tx *sql.Tx) *sql.Stmt {
	// We will never mark a visited file in our use case, hence the use
	// of visited=0.
	stmt, err := tx.Prepare(
		`UPDATE files
			SET visited=1
			WHERE path=? AND visited=0`)
	if err != nil {
		logFatal("Failed to prepare mark: %s", err.Error())
	}
	return stmt
}

func mustMarkFile(stmt *sql.Stmt, path string) {
	res, err := stmt.Exec(path)
	if err != nil {
		logFatal("Failed to mark %s: %s", path, err.Error())
	}
	assertRowsAffected(res, 1)
}

// Return nil or fileInfo.
func mustQueryFile(db *sql.DB, path string) any {
	stmt, err := db.Prepare(
		`SELECT size, checksum FROM files WHERE path=?`)
	if err != nil {
		logFatal("Failed to prepare query %s: %s", path, err.Error())
	}
	defer stmt.Close()

	ret := fileInfo{
		path:     path,
		size:     0,
		checksum: "",
	}
	var checksum any
	err = stmt.QueryRow(path).Scan(&ret.size, &checksum)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		logFatal("Failed to query %s: %s", path, err.Error())
	}

	if checksum != nil {
		ret.checksum = checksum.(string)
	}
	return ret
}

func mustQueryUnvisitedFiles(tx *sql.Tx, prefix string,
	procOneFile func(file *fileInfo)) {
	stmt, err := tx.Prepare(
		`SELECT path, size, checksum FROM files
			WHERE path LIKE ? ESCAPE '\' AND visited=0
			ORDER BY path ASC`)
	if err != nil {
		logFatal("Failed to prepare query %s: %s", prefix, err.Error())
	}
	defer stmt.Close()

	rows, err := stmt.Query(escapeForLike(prefix) + "%")
	if err != nil {
		logFatal("Failed to query %s: %s", prefix, err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		file := fileInfo{
			path:     "",
			size:     0,
			checksum: "",
		}
		var checksum any
		err = rows.Scan(&file.path, &file.size, &checksum)
		if err != nil {
			logFatal("Failed to scan %s: %s", prefix, err.Error())
		}
		if checksum != nil {
			file.checksum = checksum.(string)
		}
		procOneFile(&file)
	}
}

func mustDeleteUnvisitedFiles(tx *sql.Tx, prefix string, expectN int64) {
	stmt, err := tx.Prepare(`
		DELETE FROM files WHERE path LIKE ? ESCAPE '\' AND visited=0`)
	if err != nil {
		logFatal("Failed to prepare delete %s: %s", prefix, err.Error())
	}
	defer stmt.Close()

	res, err := stmt.Exec(escapeForLike(prefix) + "%")
	if err != nil {
		logFatal("Failed to delete %s: %s", prefix, err.Error())
	}
	assertRowsAffected(res, expectN)
}
