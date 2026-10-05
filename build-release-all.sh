#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: build-release-all.sh [OUTPUT_DIR]

Build amd64 and arm64 releases for macOS, Linux, and Windows using Go 1.20.
OUTPUT_DIR defaults to dist/ beside this script. Each ZIP contains one binary:
FolderChecksum on macOS/Linux, or FolderChecksum.exe on Windows.

Prerequisites: Go 1.20.x, zip, and these C compilers (SQLite requires CGO):

  Target          Override variable       Default compiler
  darwin/amd64    CC_DARWIN_AMD64          clang (macOS host only)
  darwin/arm64    CC_DARWIN_ARM64          clang (macOS host only)
  linux/amd64     CC_LINUX_AMD64           x86_64-linux-musl-gcc
  linux/arm64     CC_LINUX_ARM64           aarch64-linux-musl-gcc
  windows/amd64   CC_WINDOWS_AMD64         x86_64-w64-mingw32-gcc
  windows/arm64   CC_WINDOWS_ARM64         aarch64-w64-mingw32-clang

Each compiler override is an executable name or path, without arguments.
Use a wrapper script for extra compiler flags. Darwin builds require an Apple
SDK; on other hosts, both CC_DARWIN_* overrides must provide that toolchain.
Linux builds use static musl linking. Windows builds also link compiler runtime
libraries statically; use an MSVCRT-targeting amd64 compiler for Windows 7.
Windows ARM64 requires an ARM64 Windows toolchain, such as LLVM-MinGW.

Set GO_BIN to select a Go executable (default: go). For example:
  GO_BIN=/path/to/go1.20/bin/go ./build-release-all.sh

Archive names: FolderChecksum-{mac,linux,windows}-{amd64,arm64}.zip
EOF
}

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

if [[ ${1:-} == -h || ${1:-} == --help ]]; then
    usage
    exit 0
fi
[[ $# -le 1 ]] || fail 'Expected at most one output directory; see --help.'

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
output_dir=${1:-"$script_dir/dist"}
go_bin=${GO_BIN:-go}
go_bin=$(command -v "$go_bin") || fail 'Go is not installed or not on PATH; set GO_BIN to Go 1.20.'
[[ $go_bin == /* ]] || go_bin="$PWD/$go_bin"
[[ -x $go_bin ]] || fail "Go is not executable: $go_bin"
command -v zip >/dev/null || fail 'zip is required.'

go_version=$("$go_bin" env GOVERSION)
case "$go_version" in
    go1.20|go1.20.*) ;;
    *) fail "Go 1.20.x is required for Windows 7 support; found $go_version." ;;
esac

host_os=$("$go_bin" env GOHOSTOS)
if [[ $host_os != darwin ]]; then
    [[ -n ${CC_DARWIN_AMD64:-} && -n ${CC_DARWIN_ARM64:-} ]] ||
        fail 'Set CC_DARWIN_AMD64 and CC_DARWIN_ARM64 to compilers with an Apple SDK.'
fi

targets=(darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64)
compiler_vars=(CC_DARWIN_AMD64 CC_DARWIN_ARM64 CC_LINUX_AMD64 CC_LINUX_ARM64 CC_WINDOWS_AMD64 CC_WINDOWS_ARM64)
compilers=(
    "${CC_DARWIN_AMD64:-clang}"
    "${CC_DARWIN_ARM64:-clang}"
    "${CC_LINUX_AMD64:-x86_64-linux-musl-gcc}"
    "${CC_LINUX_ARM64:-aarch64-linux-musl-gcc}"
    "${CC_WINDOWS_AMD64:-x86_64-w64-mingw32-gcc}"
    "${CC_WINDOWS_ARM64:-aarch64-w64-mingw32-clang}"
)

# Check every compiler before starting a build or replacing any release files.
for i in "${!targets[@]}"; do
    compiler=$(command -v "${compilers[$i]}") ||
        fail "Missing compiler '${compilers[$i]}' for ${targets[$i]}; set ${compiler_vars[$i]}."
    [[ $compiler == /* ]] || compiler="$PWD/$compiler"
    [[ -x $compiler ]] || fail "Compiler is not executable: $compiler"
    [[ $compiler != *\"* ]] || fail 'Compiler paths cannot contain double quotes; use a wrapper on PATH.'
    compilers[$i]=$compiler
done

mkdir -p -- "$output_dir"
output_dir=$(cd -- "$output_dir" && pwd -P)
stage_dir=$(mktemp -d "$output_dir/.build-release-all.XXXXXX")
trap 'rm -rf -- "$stage_dir"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cd -- "$script_dir"
archives=()
for i in "${!targets[@]}"; do
    target=${targets[$i]}
    target_os=${target%/*}
    target_arch=${target#*/}
    archive_os=$target_os
    binary_name=FolderChecksum
    ldflags='-s -w'

    case "$target_os" in
        # Use "mac" instead of "darwin" for zip file names.
        darwin) archive_os=mac ;;
        linux|windows) ldflags+=' -linkmode=external -extldflags=-static' ;;
    esac
    if [[ $target_os == windows ]]; then
        binary_name+=.exe
    fi

    target_dir="$stage_dir/$target_os-$target_arch"
    archive_name="FolderChecksum-$archive_os-$target_arch.zip"
    mkdir -p -- "$target_dir"
    printf 'Building %s with %s\n' "$target" "${compilers[$i]}"
    # Quote CC for Go's compiler-command parser, including paths with spaces.
    CGO_ENABLED=1 GOOS="$target_os" GOARCH="$target_arch" GOAMD64=v1 \
        CC="\"${compilers[$i]}\"" \
        "$go_bin" build -mod=readonly -trimpath -buildvcs=false \
        -ldflags "$ldflags" -o "$target_dir/$binary_name" .
    (cd -- "$target_dir" && zip -q "$stage_dir/$archive_name" "$binary_name")
    archives+=("$archive_name")
done

# Publish only after all six builds and ZIP operations have succeeded.
for archive_name in "${archives[@]}"; do
    mv -f -- "$stage_dir/$archive_name" "$output_dir/$archive_name"
    printf 'Created %s\n' "$output_dir/$archive_name"
done
