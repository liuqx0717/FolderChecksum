package main

import (
	"database/sql"
	"fmt"
)

type checksumAlgorithm string

const (
	checksumAuto checksumAlgorithm = "auto"
	checksumSHA1 checksumAlgorithm = "sha1"
	checksumMD5  checksumAlgorithm = "md5"
)

func parseChecksumAlgorithm(value string) (checksumAlgorithm, error) {
	switch algorithm := checksumAlgorithm(value); algorithm {
	case checksumAuto, checksumSHA1, checksumMD5:
		return algorithm, nil
	default:
		return "", fmt.Errorf("invalid -checksum %q: expected auto, sha1, or md5", value)
	}
}

// Select one algorithm for the whole run, before any workers update the database.
// Empty checksums (including NULLs written by -sizeonly) do not identify an
// algorithm. Stop at the first nonempty checksum instead of inspecting every
// saved checksum. Use platform selection if its length is unrecognized.
func selectChecksum(db *sql.DB, requested checksumAlgorithm, acceleration func() (bool, string)) (checksumAlgorithm, string, error) {
	if requested == checksumSHA1 || requested == checksumMD5 {
		return requested, "", nil
	}
	if requested != checksumAuto {
		return "", "", fmt.Errorf("invalid -checksum %q: expected auto, sha1, or md5", requested)
	}

	var length int
	fallbackReason := "dbfile contains no saved checksums"
	err := db.QueryRow(`SELECT length(checksum) FROM files
		WHERE checksum IS NOT NULL AND checksum != '' LIMIT 1`).Scan(&length)
	if err == nil {
		switch length {
		case 32:
			return checksumMD5, "first nonempty checksum in dbfile has 32-character length (md5)", nil
		case 40:
			return checksumSHA1, "first nonempty checksum in dbfile has 40-character length (sha1)", nil
		default:
			fallbackReason = fmt.Sprintf("first nonempty checksum in dbfile has unrecognized length %d", length)
		}
	} else if err != sql.ErrNoRows {
		return "", "", fmt.Errorf("failed to detect checksum algorithm from dbfile: %w", err)
	}
	accelerated, reason := acceleration()
	if accelerated {
		return checksumSHA1, fallbackReason + "; " + reason, nil
	}
	return checksumMD5, fallbackReason + "; " + reason, nil
}

func mustSelectChecksum(cfg *config) {
	if cfg.sizeOnly {
		logInfo("Checksum calculation disabled by -sizeonly")
		return
	}
	algorithm, reason, err := selectChecksum(cfg.db, cfg.checksum, sha1Acceleration)
	if err != nil {
		logFatal("%s", err)
	}
	if cfg.checksum == checksumAuto {
		logInfo("Using checksum algorithm: %s (auto: %s)", algorithm, reason)
	}
	cfg.checksum = algorithm
}
