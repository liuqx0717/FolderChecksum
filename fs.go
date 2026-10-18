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
// Symlinks in rootDir and prefix are resolved normally to reach the starting
// path. Symlinks encountered during traversal are skipped unless followLinks
// is true; following a broken link or directory cycle fails the scan.
// Report paths through the link, not the target.
func mustWalkDir(rootDir string, prefix string, followLinks bool,
	procOneFile func(relPath string, size int64)) {
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
	// Resolve the explicitly selected starting path regardless of followLinks.
	info, err := fs.Stat(fsys, prefixArg)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			// A missing prefix (including a regular-file ancestor) is allowed
			// so its old records can be deleted.
			logWarning("Failed to stat prefix '%s', skipped", prefixArg)
			return
		}
		logFatal("Failed to walk '%s': %s", prefixArg, err.Error())
	}

	var walk func(string, fs.FileInfo, []fs.FileInfo)
	walk = func(relPath string, info fs.FileInfo, ancestors []fs.FileInfo) {
		if followLinks && info.Mode()&fs.ModeSymlink != 0 {
			var err error
			info, err = fs.Stat(fsys, relPath)
			if err != nil {
				logFatal("Failed to follow symlink '%s': %s", relPath, err.Error())
			}
		}
		logDebug("Found path=%s, isDir=%v, isSpecial=%v",
			relPath, info.IsDir(), isSpecialFile(info.Mode()))

		if !info.IsDir() {
			if !isSpecialFile(info.Mode()) {
				procOneFile(relPath, info.Size())
			}
			return
		}
		if followLinks {
			// Track only the current ancestry: separate aliases of the same
			// directory must each be visited under their own relative paths.
			for _, ancestor := range ancestors {
				if os.SameFile(info, ancestor) {
					logFatal("Directory cycle at '%s'", relPath)
				}
			}
			ancestors = append(ancestors, info)
		}

		// ReadDir preserves the lexical order used by fs.WalkDir.
		entries, err := fs.ReadDir(fsys, relPath)
		if err != nil {
			logFatal("Failed to walk '%s': %s", relPath, err.Error())
		}
		for _, entry := range entries {
			childPath := path.Join(relPath, entry.Name())
			childInfo, err := entry.Info()
			if err != nil {
				logFatal("Failed to stat '%s': %s", childPath, err.Error())
			}
			walk(childPath, childInfo, ancestors)
		}
	}
	walk(prefixArg, info, nil)
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
