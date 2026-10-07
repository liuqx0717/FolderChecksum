package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestParseChecksumAlgorithm(t *testing.T) {
	for _, value := range []string{"auto", "sha1", "md5"} {
		algorithm, err := parseChecksumAlgorithm(value)
		if err != nil || string(algorithm) != value {
			t.Fatalf("parse %q: got %q, %v", value, algorithm, err)
		}
	}
	for _, value := range []string{"", "SHA1", "sha256", "sha-1", " md5"} {
		if _, err := parseChecksumAlgorithm(value); err == nil {
			t.Errorf("accepted invalid algorithm %q", value)
		}
	}
}

func TestSelectChecksum(t *testing.T) {
	md5 := strings.Repeat("a", 32)
	sha1 := strings.Repeat("b", 40)
	tests := []struct {
		name        string
		checksums   []any
		accelerated bool
		wantCalls   int
		want        checksumAlgorithm
		reason      string
		dbReason    string
	}{
		{name: "empty accelerated", accelerated: true, wantCalls: 1, want: checksumSHA1, reason: "test platform has an accelerated SHA-1 implementation"},
		{name: "empty unaccelerated", wantCalls: 1, want: checksumMD5, reason: "test platform uses the MD5 fallback"},
		{name: "size only accelerated", checksums: []any{nil, ""}, accelerated: true, wantCalls: 1, want: checksumSHA1, reason: "test platform has an accelerated SHA-1 implementation"},
		{name: "size only unaccelerated", checksums: []any{nil, ""}, wantCalls: 1, want: checksumMD5, reason: "test platform uses the MD5 fallback"},
		{name: "md5 overrides CPU", checksums: []any{nil, "", md5, md5}, accelerated: true, want: checksumMD5, reason: "32-character"},
		{name: "sha1 overrides CPU", checksums: []any{nil, "", sha1, sha1}, want: checksumSHA1, reason: "40-character"},
		{name: "mixed uses first md5", checksums: []any{nil, "", md5, sha1}, accelerated: true, want: checksumMD5, reason: "32-character"},
		{name: "mixed uses first sha1", checksums: []any{nil, "", sha1, md5}, want: checksumSHA1, reason: "40-character"},
		{name: "unknown accelerated", checksums: []any{"abc"}, accelerated: true, wantCalls: 1, want: checksumSHA1, reason: "test platform has an accelerated SHA-1 implementation", dbReason: "first nonempty checksum in dbfile has unrecognized length 3"},
		{name: "unknown unaccelerated", checksums: []any{"abc"}, wantCalls: 1, want: checksumMD5, reason: "test platform uses the MD5 fallback", dbReason: "first nonempty checksum in dbfile has unrecognized length 3"},
		{name: "unknown before md5 accelerated", checksums: []any{nil, "", "abc", md5}, accelerated: true, wantCalls: 1, want: checksumSHA1, reason: "test platform has an accelerated SHA-1 implementation", dbReason: "first nonempty checksum in dbfile has unrecognized length 3"},
		{name: "unknown before sha1 unaccelerated", checksums: []any{nil, "", "abc", sha1}, wantCalls: 1, want: checksumMD5, reason: "test platform uses the MD5 fallback", dbReason: "first nonempty checksum in dbfile has unrecognized length 3"},
		{name: "unknown after md5 ignored", checksums: []any{md5, "abc"}, want: checksumMD5, reason: "32-character"},
		{name: "unknown after sha1 ignored", checksums: []any{sha1, "abc"}, want: checksumSHA1, reason: "40-character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := prepareTestDb(t)
			defer db.Close()
			for i, checksum := range tt.checksums {
				if _, err := db.Exec("INSERT INTO files VALUES (?, 0, ?, 0)", i, checksum); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			got, reason, err := selectChecksum(db, checksumAuto, func() (bool, string) {
				calls++
				return tt.accelerated, tt.reason
			})
			if calls != tt.wantCalls {
				t.Fatalf("platform selection called %d times, want %d", calls, tt.wantCalls)
			}
			if err != nil || got != tt.want || !strings.Contains(reason, tt.reason) {
				t.Fatalf("got (%q, %q, %v), want %q and reason containing %q", got, reason, err, tt.want, tt.reason)
			}
			if tt.wantCalls != 0 {
				dbReason := tt.dbReason
				if dbReason == "" {
					dbReason = "dbfile contains no saved checksums"
				}
				if reason != dbReason+"; "+tt.reason {
					t.Errorf("database and platform selection reasons were not preserved: %q", reason)
				}
			}
		})
	}
}

func TestSelectChecksumExplicit(t *testing.T) {
	// Explicit selection must not inspect the database or detect the platform.
	for _, algorithm := range []checksumAlgorithm{checksumMD5, checksumSHA1} {
		got, _, err := selectChecksum(nil, algorithm, nil)
		if err != nil || got != algorithm {
			t.Fatalf("select %s: got %s, %v", algorithm, got, err)
		}
	}
	if _, _, err := selectChecksum(nil, "invalid", nil); err == nil {
		t.Fatal("accepted an invalid algorithm")
	}
}

func TestSelectChecksumDatabaseError(t *testing.T) {
	db := prepareTestDb(t)
	db.Close()
	calls := 0
	if _, _, err := selectChecksum(db, checksumAuto, func() (bool, string) {
		calls++
		return true, "test acceleration"
	}); err == nil {
		t.Fatal("database error was treated as a reason for platform selection")
	}
	if calls != 0 {
		t.Fatalf("platform selection called %d times after a database error", calls)
	}
}

func TestAutoChecksumLog(t *testing.T) {
	db := prepareTestDb(t)
	defer db.Close()
	if _, err := db.Exec("INSERT INTO files VALUES ('file', 0, ?, 0)", strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	previousOutput, previousLevel := log.Writer(), logLevel
	log.SetOutput(&output)
	logLevel = INFO
	defer func() {
		log.SetOutput(previousOutput)
		logLevel = previousLevel
	}()
	cfg := config{db: db, checksum: checksumAuto}
	mustSelectChecksum(&cfg)
	if cfg.checksum != checksumMD5 || !strings.Contains(output.String(), "Using checksum algorithm: md5 (auto:") || !strings.Contains(output.String(), "32-character") {
		t.Fatalf("missing selected algorithm or reason: %s", &output)
	}
}

func TestSizeOnlySkipsChecksumSelection(t *testing.T) {
	// No database is necessary when checksums will not be calculated.
	cfg := config{checksum: checksumAuto, sizeOnly: true}
	mustSelectChecksum(&cfg)
}
