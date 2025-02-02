package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
)

var stats struct {
	numFilesNew            atomic.Int64
	numFilesChanged        atomic.Int64
	numFilesDeleted        atomic.Int64
	numFilesUnchanged      atomic.Int64
	numVisitedFlagsCleared atomic.Int64
}

type fileCheckMsg struct {
	relPath string
	size    int64
}

type dbUpdateMsg struct {
	opType string
	info   fileInfo
}

func clearStats() {
	stats.numFilesNew.Store(0)
	stats.numFilesChanged.Store(0)
	stats.numFilesDeleted.Store(0)
	stats.numFilesUnchanged.Store(0)
	stats.numVisitedFlagsCleared.Store(0)
}

func outputNewFile(cfg *config, relPath string) {
	fmt.Fprintln(cfg.outFile, "new:", relPath)
	stats.numFilesNew.Add(1)
}

func outputChangedFile(cfg *config, relPath string) {
	fmt.Fprintln(cfg.outFile, "changed:", relPath)
	stats.numFilesChanged.Add(1)
}

func outputDeletedFile(cfg *config, relPath string) {
	fmt.Fprintln(cfg.outFile, "deleted:", relPath)
	stats.numFilesDeleted.Add(1)
}

func outputUnchangedFile(cfg *config, relPath string) {
	logDebug("unchanged: %s", relPath)
	stats.numFilesUnchanged.Add(1)
}

func mustCalcChecksum(path string, size int64, sizeOnly bool) string {
	if sizeOnly {
		return ""
	}
	checksum, n := mustCalcFileMd5(path)
	if n != size {
		logFatal("Failed to checksum '%s': size=%d, n=%d", path, size, n)
	}
	return checksum
}

func shouldExcludePath(cfg *config, relPath string) bool {
	return cfg.excludeRe.MatchString(relPath) &&
		!cfg.includeRe.MatchString(relPath)
}

func fileCheckWorker(id int, cfg *config, wg *sync.WaitGroup,
	cIn <-chan fileCheckMsg, cOut chan<- dbUpdateMsg) {
	// This worker doesn't create any tx on its own.
	logDebug("Started fileCheckWorker %d", id)

	for msg := range cIn {
		path := filepath.Join(cfg.rootDir, msg.relPath)
		if shouldExcludePath(cfg, msg.relPath) {
			logInfo("skipped: %s", msg.relPath)
			continue
		}

		infoInDb, _ := mustQueryFile(cfg.db, msg.relPath)
		info := fileInfo{
			relPath:  msg.relPath,
			size:     msg.size,
			checksum: "",
		}

		logDebug("(worker %d) checking %s: %+v", id, msg.relPath, infoInDb)

		if infoInDb == nil {
			// Db doesn't have this file.
			outputNewFile(cfg, msg.relPath)
			if cfg.update {
				info.checksum = mustCalcChecksum(path, msg.size, cfg.sizeOnly)
				// Insert the file into db.
				cOut <- dbUpdateMsg{"I", info}
			}
			continue
		}

		if infoInDb.(fileInfo).size != msg.size {
			// Db has this file, but size is different.
			outputChangedFile(cfg, msg.relPath)
			if cfg.update {
				info.checksum = mustCalcChecksum(path, msg.size, cfg.sizeOnly)
				// Update the file in db.
				cOut <- dbUpdateMsg{"U", info}
			} else {
				// Mark the file visited.
				cOut <- dbUpdateMsg{"M", info}
			}
			continue
		}

		dbHasChecksum := infoInDb.(fileInfo).checksum != ""

		// Db has this file, size is the same. The file is deemed unchanged
		// in sizeOnly mode.
		if cfg.sizeOnly {
			outputUnchangedFile(cfg, msg.relPath)
			if dbHasChecksum && cfg.update {
				// Clear the original checksum in db.
				cOut <- dbUpdateMsg{"U", info}
			} else {
				// Mark the file visited.
				cOut <- dbUpdateMsg{"M", info}
			}
			continue
		}

		// Compare the checksum.
		info.checksum = mustCalcChecksum(path, msg.size, cfg.sizeOnly)
		if !dbHasChecksum {
			logWarning("Db only has size info for '%s' but -sizeonly is "+
				"not used.", msg.relPath)
		}
		if infoInDb.(fileInfo).checksum == info.checksum {
			outputUnchangedFile(cfg, msg.relPath)
			// Mark the file visited.
			cOut <- dbUpdateMsg{"M", info}
		} else {
			outputChangedFile(cfg, msg.relPath)
			if cfg.update {
				// Update the file in db.
				cOut <- dbUpdateMsg{"U", info}
			} else {
				// Mark the file visited.
				cOut <- dbUpdateMsg{"M", info}
			}
		}
	}

	logDebug("Stopped fileCheckWorker %d", id)
	wg.Done()
}

func verifyStats(cfg *config) {
	numFilesNew := stats.numFilesNew.Load()
	numFilesChanged := stats.numFilesChanged.Load()
	numFilesDeleted := stats.numFilesDeleted.Load()
	numFilesUnchanged := stats.numFilesUnchanged.Load()
	numVisitedFlagsCleared := stats.numVisitedFlagsCleared.Load()

	logInfo("stats: numFilesNew=%d numFilesChanged=%d "+
		"numFilesDeleted=%d numFilesUnchanged=%d numVisitedFlagsCleared=%d",
		numFilesNew, numFilesChanged, numFilesDeleted,
		numFilesUnchanged, numVisitedFlagsCleared)

	if cfg.update {
		if numVisitedFlagsCleared !=
			numFilesNew+numFilesChanged+numFilesUnchanged {
			logFatal("stats inconsistent: numVisitedFlagsCleared=%d, "+
				"numFilesNew+numFilesChanged+numFilesUnchanged=%d",
				numVisitedFlagsCleared,
				numFilesNew+numFilesChanged+numFilesUnchanged)
		}
	} else {
		if numVisitedFlagsCleared != 0 {
			logFatal("numVisitedFlagsCleared=%d, expected 0",
				numVisitedFlagsCleared)
		}
	}
}

// Output the unvisited files as deleted files, remove them from db, then
// clear the "visited" flag in db.
// Note that we can't use range query alone. Consider this case: the db
// contains one record for a normal file named "aa", and multiple records
// for the files under folder "aab/". Then the user use "aa" as the prefix.
// We should first check the prefix itself ("aa"), then use range query on
// the prefix with a trailing slash ("aa/").
func mustHandleDeletedFiles(cfg *config, tx *sql.Tx, prefix string) {
	numDeleted := int64(0)
	numCleared := int64(0)
	procUnvisitedFile := func(file *fileInfo) {
		outputDeletedFile(cfg, file.relPath)
		numDeleted++
	}

	if prefix == "" {
		// Process all entries.
		mustQueryUnvisitedFiles(tx, "", procUnvisitedFile)
		if cfg.update {
			mustDeleteUnvisitedFiles(tx, "", numDeleted)
			numCleared = mustClearVisitedFlags(tx, "")
			stats.numVisitedFlagsCleared.Add(numCleared)
		}
		return
	}

	if prefix[len(prefix)-1] == '/' {
		logFatal("prefix '%s' has a trailing slash", prefix)
	}

	// Process "prefix"
	file, visited := mustQueryFile(tx, prefix)
	if file != nil {
		f := file.(fileInfo)
		if visited {
			if cfg.update {
				mustClearVisitedFlag(tx, prefix)
				stats.numVisitedFlagsCleared.Add(1)
			}
		} else {
			procUnvisitedFile(&f)
			if cfg.update {
				mustDeleteUnvisitedFile(tx, prefix)
				numDeleted = 0
			}
		}
	}

	// Process "prefix/..."
	mustQueryUnvisitedFiles(tx, prefix+"/", procUnvisitedFile)
	if cfg.update {
		mustDeleteUnvisitedFiles(tx, prefix+"/", numDeleted)
		numCleared = mustClearVisitedFlags(tx, prefix+"/")
		stats.numVisitedFlagsCleared.Add(numCleared)
	}
}

func dbUpdateWorker(cfg *config, wg *sync.WaitGroup,
	cIn <-chan dbUpdateMsg) {
	// This worker creates a tx on its own. All db APIs should use it.
	// I.e., don't use db.Prepare(), db.Exec(), etc.
	logDebug("Started dbUpdateWorker")
	tx := mustCreateTx(cfg.db)
	insStmt := mustPrepareInsertFile(tx)
	updStmt := mustPrepareUpdateAndMarkFile(tx)
	mrkStmt := mustPrepareMarkFile(tx)

	for msg := range cIn {
		logDebug("updating: %+v", msg)
		switch msg.opType {
		case "I":
			mustInsertFile(insStmt, &msg.info)
		case "U":
			mustUpdateAndMarkFile(updStmt, &msg.info)
		case "M":
			mustMarkFile(mrkStmt, msg.info.relPath)
		case "D":
			mustHandleDeletedFiles(cfg, tx, msg.info.relPath)
		default:
			logFatal("Unknown opType %s", msg.opType)
		}
	}

	verifyStats(cfg)
	if cfg.update {
		mustCommitTx(tx)
	} else {
		tx.Rollback()
	}

	logDebug("Stopped dbUpdateWorker")
	wg.Done()
}
