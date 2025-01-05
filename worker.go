package main

import (
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

func outputNewFile(relPath string) {
	fmt.Println("new:", relPath)
	logDebug("new: %s", relPath)
	stats.numFilesNew.Add(1)
}

func outputChangedFile(relPath string) {
	fmt.Println("changed:", relPath)
	logDebug("changed: %s", relPath)
	stats.numFilesChanged.Add(1)
}

func outputDeletedFile(relPath string) {
	fmt.Println("deleted:", relPath)
	logDebug("deleted: %s", relPath)
	stats.numFilesDeleted.Add(1)
}

func outputUnchangedFile(relPath string) {
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

func fileCheckWorker(cfg *config, wg *sync.WaitGroup,
	cIn <-chan fileCheckMsg, cOut chan<- dbUpdateMsg) {
	// This worker doesn't create any tx on its own.
	logDebug("Started fileCheckWorker")

	for msg := range cIn {
		path := filepath.Join(cfg.rootDir, msg.relPath)
		infoInDb, _ := mustQueryFile(cfg.db, msg.relPath)
		info := fileInfo{
			relPath:  msg.relPath,
			size:     msg.size,
			checksum: "",
		}

		logDebug("checking %s: %+v", path, infoInDb)

		if infoInDb == nil {
			// Db doesn't have this file.
			outputNewFile(msg.relPath)
			if cfg.update {
				info.checksum = mustCalcChecksum(path, msg.size, cfg.sizeOnly)
				// Insert the file into db.
				cOut <- dbUpdateMsg{"I", info}
			}
			continue
		}

		if infoInDb.(fileInfo).size != msg.size {
			// Db has this file, but size is different.
			outputChangedFile(msg.relPath)
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
			outputUnchangedFile(msg.relPath)
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
			logWarning("Db only has size info for '%s' but -sizeonly is " +
				"not used.")
		}
		if infoInDb.(fileInfo).checksum == info.checksum {
			outputUnchangedFile(msg.relPath)
			// Mark the file visited.
			cOut <- dbUpdateMsg{"M", info}
		} else {
			outputChangedFile(msg.relPath)
			if cfg.update {
				// Update the file in db.
				cOut <- dbUpdateMsg{"U", info}
			} else {
				// Mark the file visited.
				cOut <- dbUpdateMsg{"M", info}
			}
		}
	}

	logDebug("Stopped fileCheckWorker")
	wg.Done()
}
