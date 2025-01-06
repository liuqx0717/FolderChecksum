package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// Return false for directories and regular files. Return true otherwise.
func isSpecialFile(mode fs.FileMode) bool {
	// Clear ModeDir bit from ModeType.
	specialBits := fs.ModeType &^ fs.ModeDir
	return (mode & specialBits) != 0
}

func cleanPrefix(prefix string) string {
	// If prefix contains '..', the result of path.Clean() could be
	// something like '..' or '../..'. So we prepend it with '/' to
	// squash the excess '..', then remove the leading '/'.
	return path.Clean("/" + prefix)[1:]
}

func dirMustExist(path string) {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		logFatal("Failed to evaluate '%s' :%s", path, err.Error())
	}
	info, err := os.Stat(realPath)
	if err != nil {
		logFatal("Failed to stat '%s' :%s", realPath, err.Error())
	}
	if !info.IsDir() {
		logFatal("Not a folder '%s' :%s", realPath, err.Error())
	}
}

// Recursively enumerate all the files under rootDir whose relative
// path starts with prefix. Call procOneFile with the path relative
// to rootDir and the file size. procOneFile is NOT called on folders.
// Slash (/) is always used as path separator in prefix and relPath,
// even on Windows.
//
// rootDir must be an existing directory. If prefix doesn't exist,
// this function will return (without failing).
//
// By default symlinks in rootDir and prefix are followed and others
// are skipped. When followSymLinks is true, follow all the links.
func mustWalkDir(rootDir string, prefix string, followLinks bool,
	procOneFile func(relPath string, size int64)) {
	if followLinks {
		logFatal("followSymLinks not implemented")
	}

	dirMustExist(rootDir)

	// If prefix contains '..', the result of path.Clean() could be
	// something like '..' or '../..'. So we prepend it with '/' to
	// squash the excess '..', then remove the leading '/'.
	prefixArg := cleanPrefix(prefix)
	if prefixArg == "" {
		prefixArg = "."
	}
	logDebug("WalkDir rootDir=%s, prefix=%s prefixArg=%s",
		rootDir, prefix, prefixArg)

	fsys := os.DirFS(rootDir)
	fs.WalkDir(fsys, prefixArg,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d == nil {
					// The initial fs.Stat failed.
					logWarning("Failed to stat prefix '%s', skipped", path)
					return nil
				}
				// A directory's ReadDir method failed.
				logFatal("Failed to walk '%s': %s", path, err.Error())
			}
			isDir := d.IsDir()
			mode := d.Type()
			info, err := d.Info()
			if err != nil {
				logFatal("Failed to stat '%s': %s", path, err.Error())
			}
			logDebug("Found path=%s, isDir=%v, isSpecial=%v",
				path, isDir, isSpecialFile(mode))

			if !isDir && !isSpecialFile(mode) {
				procOneFile(path, info.Size())
			}
			return nil
		})
}

// Return md5 string and number of bytes read.
func mustCalcFileMd5(filePath string) (string, int64) {
	file, err := os.Open(filePath)
	if err != nil {
		logFatal("Failed to open '%s': %s", filePath, err.Error())
	}
	defer file.Close()

	hash := md5.New()
	n, err := io.Copy(hash, file)
	if err != nil {
		logFatal("Failed to compute md5 for '%s': %s", filePath, err.Error())
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), n
}
