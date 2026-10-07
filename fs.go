package main

import (
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"
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
				if d == nil && (os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR)) {
					// A missing prefix (including a regular-file ancestor) is allowed
					// so its old records can be deleted.
					logWarning("Failed to stat prefix '%s', skipped", path)
					return nil
				}
				// Other stat or ReadDir errors must not look like deleted files.
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

// Return the hexadecimal checksum and number of bytes read.
func mustCalcFileChecksum(filePath string, algorithm checksumAlgorithm) (string, int64) {
	var digest hash.Hash
	switch algorithm {
	case checksumMD5:
		digest = md5.New()
	case checksumSHA1:
		digest = sha1.New()
	default:
		logFatal("Unknown checksum algorithm: %s", algorithm)
	}

	file, err := os.Open(filePath)
	if err != nil {
		logFatal("Failed to open '%s': %s", filePath, err.Error())
	}
	defer file.Close()

	n, err := io.Copy(digest, file)
	if err != nil {
		logFatal("Failed to compute %s for '%s': %s", algorithm, filePath, err.Error())
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), n
}
