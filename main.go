package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Parse()
	if cfg.version {
		fmt.Printf("Version %s\n", VERSION)
		os.Exit(0)
	}
	if cfg.followLinks {
		logFatal("Option not implemented")
	}
}
