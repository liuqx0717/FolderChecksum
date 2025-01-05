package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
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
	cfg.db.Close()
}
