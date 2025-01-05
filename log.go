package main

import (
	"log"
	"os"
)

const (
	ERROR   = 0
	WARNING = 1
	INFO    = 2
	DEBUG   = 3
)

var logLevel int

func logFatal(fmt string, args ...any) {
	log.Printf("[ERROR] "+fmt, args...)
	os.Exit(1)
}

func logWarning(fmt string, args ...any) {
	if logLevel >= WARNING {
		log.Printf("[WARN]  "+fmt, args...)
	}
}

func logInfo(fmt string, args ...any) {
	if logLevel >= INFO {
		log.Printf("[INFO]  "+fmt, args...)
	}
}

func logDebug(fmt string, args ...any) {
	if logLevel >= DEBUG {
		log.Printf("[DEBUG] "+fmt, args...)
	}
}

func init() {
	log.SetFlags(log.Ltime)
}
