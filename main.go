package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
)

func main() {
	// Parse arguments.
	flag.Parse()
	if flg.version {
		fmt.Printf("Version %s\n", VERSION)
		os.Exit(0)
	}
	parsePositionalArgs()
	cfg := flagsToConfig(&flg)
	if cfg.followLinks {
		logFatal("Option not implemented")
	}
	mustCreateFilesTableIfNeeded(cfg.db)

	// Start workers (1 dbUpdateWorker and j fileCheckWorker).
	chFileCheck := make(chan fileCheckMsg)
	chDbUpdate := make(chan dbUpdateMsg, 128)
	var wgDbUpdate sync.WaitGroup
	var wgFileCheck sync.WaitGroup
	wgDbUpdate.Add(1)
	wgFileCheck.Add(cfg.j)
	go dbUpdateWorker(cfg, &wgDbUpdate, chDbUpdate)
	for i := 0; i < cfg.j; i++ {
		go fileCheckWorker(i+1, cfg, &wgFileCheck, chFileCheck, chDbUpdate)
	}

	// Walk the folder.
	procOneFile := func(relPath string, size int64) {
		chFileCheck <- fileCheckMsg{relPath, size}
	}
	if len(cfg.prefix) == 0 {
		mustWalkDir(cfg.rootDir, "", cfg.followLinks, procOneFile)
	} else {
		for _, prefix := range cfg.prefix {
			mustWalkDir(cfg.rootDir, prefix, cfg.followLinks, procOneFile)
		}
	}

	// Wait for fileCheckWorker.
	close(chFileCheck)
	wgFileCheck.Wait()

	// Check the deleted files.
	if len(cfg.prefix) == 0 {
		chDbUpdate <- dbUpdateMsg{"D", fileInfo{"", 0, ""}}
	} else {
		for _, prefix := range cfg.prefix {
			chDbUpdate <- dbUpdateMsg{"D", fileInfo{prefix, 0, ""}}
		}
	}

	// Wait for dbUpdateWorker.
	close(chDbUpdate)
	wgDbUpdate.Wait()

	cfg.db.Close()
}
