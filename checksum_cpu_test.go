package main

import (
	"strings"
	"testing"
)

func TestSHA1AccelerationForPlatform(t *testing.T) {
	allFeatures := checksumFeatures{avx2: true, bmi1: true, bmi2: true, sha1: true}
	tests := []struct {
		name     string
		goos     string
		goarch   string
		features checksumFeatures
		debug    string
		want     bool
		reason   string
	}{
		{"macOS amd64", "darwin", "amd64", allFeatures, "", true, "AVX2 path is available"},
		{"macOS arm64", "darwin", "arm64", allFeatures, "", true, "ARM SHA-1 instructions"},
		{"Linux amd64", "linux", "amd64", allFeatures, "", true, "AVX2 path is available"},
		{"Linux arm64", "linux", "arm64", allFeatures, "", true, "ARM SHA-1 instructions"},
		{"Windows amd64", "windows", "amd64", allFeatures, "", true, "AVX2 path is available"},
		{"Windows arm64 ignores hardware", "windows", "arm64", allFeatures, "", false, "generic implementation"},
		{"amd64 needs no SHA instructions", "linux", "amd64", checksumFeatures{avx2: true, bmi1: true, bmi2: true}, "", true, "AVX2 path is available"},
		{"amd64 missing AVX2", "linux", "amd64", checksumFeatures{bmi1: true, bmi2: true, sha1: true}, "", false, "unavailable or disabled: AVX2"},
		{"amd64 missing BMI1", "linux", "amd64", checksumFeatures{avx2: true, bmi2: true, sha1: true}, "", false, "unavailable or disabled: BMI1"},
		{"amd64 missing BMI2", "linux", "amd64", checksumFeatures{avx2: true, bmi1: true, sha1: true}, "", false, "unavailable or disabled: BMI2"},
		{"amd64 no features", "linux", "amd64", checksumFeatures{}, "", false, "unavailable or disabled: AVX2, BMI1, BMI2"},
		{"amd64 disable AVX2", "linux", "amd64", allFeatures, "cpu.avx2=off", false, "unavailable or disabled: AVX2"},
		{"amd64 disable BMI1", "linux", "amd64", allFeatures, "cpu.bmi1=off", false, "unavailable or disabled: BMI1"},
		{"amd64 disable BMI2", "linux", "amd64", allFeatures, "cpu.bmi2=off", false, "unavailable or disabled: BMI2"},
		{"amd64 disable all", "linux", "amd64", allFeatures, "cpu.all=off", false, "unavailable or disabled: AVX2, BMI1, BMI2"},
		{"amd64 reenable one insufficient", "linux", "amd64", allFeatures, "cpu.all=off,cpu.avx2=on", false, "unavailable or disabled: BMI1, BMI2"},
		{"amd64 reenable required features", "linux", "amd64", allFeatures, "cpu.all=off,cpu.avx2=on,cpu.bmi1=on,cpu.bmi2=on", true, "AVX2 path is available"},
		{"amd64 dedicated SHA irrelevant", "linux", "amd64", allFeatures, "cpu.sha=off", true, "AVX2 path is available"},
		{"ARM missing SHA1", "linux", "arm64", checksumFeatures{}, "", false, "unavailable or disabled"},
		{"ARM disable SHA1", "darwin", "arm64", allFeatures, "cpu.sha1=off", false, "unavailable or disabled"},
		{"ARM disable all", "linux", "arm64", allFeatures, "cpu.all=off", false, "unavailable or disabled"},
		{"ARM reenable SHA1", "linux", "arm64", allFeatures, "cpu.all=off,cpu.sha1=on", true, "ARM SHA-1 instructions"},
		{"Android arm64", "android", "arm64", allFeatures, "", true, "ARM SHA-1 instructions"},
		{"FreeBSD arm64", "freebsd", "arm64", allFeatures, "", true, "ARM SHA-1 instructions"},
		{"OpenBSD arm64", "openbsd", "arm64", allFeatures, "", true, "ARM SHA-1 instructions"},
		{"iOS arm64", "ios", "arm64", allFeatures, "", false, "generic implementation"},
		{"other ARM OS", "netbsd", "arm64", allFeatures, "", false, "generic implementation"},
		{"386 ignores x86 features", "linux", "386", allFeatures, "", false, "no supported Go 1.20 SHA-1 acceleration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := sha1AccelerationForPlatform(tt.goos, tt.goarch, tt.features, tt.debug)
			if got != tt.want {
				t.Fatalf("got acceleration %v, want %v (reason: %s)", got, tt.want, reason)
			}
			if !strings.Contains(reason, tt.goos+"/"+tt.goarch) || !strings.Contains(reason, tt.reason) {
				t.Fatalf("reason %q must include platform and %q", reason, tt.reason)
			}
		})
	}
}

func TestSHA1FeatureEnabled(t *testing.T) {
	tests := []struct {
		name     string
		detected bool
		feature  string
		debug    string
		want     bool
	}{
		{"available", true, "sha1", "", true},
		{"unavailable", false, "sha1", "", false},
		{"disable ARM SHA1", true, "sha1", "cpu.sha1=off", false},
		{"disable x86 SHA", true, "sha", "cpu.sha=off", false},
		{"disable all", true, "sha1", "cpu.all=off", false},
		{"reenable one", true, "sha1", "cpu.all=off,cpu.sha1=on", true},
		{"disable all last", true, "sha1", "cpu.sha1=on,cpu.all=off", false},
		{"reenable all", true, "sha", "cpu.sha=off,cpu.all=on", true},
		{"cannot enable absent hardware", false, "sha", "cpu.sha=on", false},
		{"unrelated features", true, "sha1", "cpu.sha=off,gctrace=1", true},
		{"invalid options ignored", true, "sha1", "cpu.sha1,cpu.sha1=invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sha1FeatureEnabled(tt.detected, tt.feature, tt.debug); got != tt.want {
				t.Fatalf("sha1FeatureEnabled(%v, %q, %q) = %v, want %v", tt.detected, tt.feature, tt.debug, got, tt.want)
			}
		})
	}
}

func TestSHA1AccelerationDisabled(t *testing.T) {
	t.Setenv("GODEBUG", "cpu.all=off")
	if hasSHA1Acceleration() {
		t.Fatal("hardware acceleration enabled despite GODEBUG=cpu.all=off")
	}
}
