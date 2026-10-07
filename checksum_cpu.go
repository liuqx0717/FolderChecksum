package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// hasSHA1Acceleration reports whether Go 1.20 can use an accelerated SHA-1
// implementation on the current OS, architecture, and CPU.
func hasSHA1Acceleration() bool {
	accelerated, _ := sha1Acceleration()
	return accelerated
}

func sha1Acceleration() (bool, string) {
	return sha1AccelerationForPlatform(runtime.GOOS, runtime.GOARCH, checksumCPUFeatures(), os.Getenv("GODEBUG"))
}

type checksumFeatures struct {
	avx2 bool
	bmi1 bool
	bmi2 bool
	sha1 bool
}

// Match the paths used by Go 1.20's crypto/sha1. In particular, its amd64
// implementation uses AVX2 rather than dedicated x86 SHA instructions, and
// Windows ARM64 cannot use the ARM SHA-1 path even on capable hardware.
func sha1AccelerationForPlatform(goos, goarch string, features checksumFeatures, debug string) (bool, string) {
	platform := goos + "/" + goarch
	switch goarch {
	case "amd64":
		var unavailable []string
		for _, feature := range []struct {
			name     string
			detected bool
		}{
			{"avx2", features.avx2},
			{"bmi1", features.bmi1},
			{"bmi2", features.bmi2},
		} {
			if !sha1FeatureEnabled(feature.detected, feature.name, debug) {
				unavailable = append(unavailable, strings.ToUpper(feature.name))
			}
		}
		if len(unavailable) == 0 {
			return true, platform + ": Go 1.20 SHA-1 AVX2 path is available (AVX2, BMI1, and BMI2 enabled)"
		}
		return false, fmt.Sprintf("%s: Go 1.20 SHA-1 AVX2 path requires AVX2, BMI1, and BMI2; unavailable or disabled: %s; using MD5 fallback", platform, strings.Join(unavailable, ", "))
	case "arm64":
		switch goos {
		case "darwin", "linux", "android", "freebsd", "openbsd":
			if sha1FeatureEnabled(features.sha1, "sha1", debug) {
				return true, platform + ": Go 1.20 uses enabled ARM SHA-1 instructions"
			}
			return false, platform + ": ARM SHA-1 instructions are unavailable or disabled; using MD5 fallback"
		default:
			return false, platform + ": Go 1.20 SHA-1 uses the generic implementation; using MD5 fallback"
		}
	default:
		return false, platform + ": no supported Go 1.20 SHA-1 acceleration was detected; using MD5 fallback"
	}
}

// cpuid does not read GODEBUG. Respect the same feature overrides as Go's CPU
// detection; later settings take precedence, and cannot enable absent hardware.
func sha1FeatureEnabled(detected bool, feature, debug string) bool {
	enabled := true
	for _, setting := range strings.Split(debug, ",") {
		name, value, ok := strings.Cut(setting, "=")
		if !ok || (name != "cpu.all" && name != "cpu."+feature) {
			continue
		}
		switch value {
		case "on":
			enabled = true
		case "off":
			enabled = false
		}
	}
	return detected && enabled
}
