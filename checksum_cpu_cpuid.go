package main

import "github.com/klauspost/cpuid/v2"

func checksumCPUFeatures() checksumFeatures {
	// cpuid's AVX2 detection includes the OS's support for saving YMM registers.
	// Unsupported platforms conservatively report no detected acceleration.
	return checksumFeatures{
		avx2: cpuid.CPU.Supports(cpuid.AVX2),
		bmi1: cpuid.CPU.Supports(cpuid.BMI1),
		bmi2: cpuid.CPU.Supports(cpuid.BMI2),
		sha1: cpuid.CPU.Supports(cpuid.SHA1),
	}
}
