# Overview

This tool saves the current information of a folder to a database file.
Later the database file can be used to track the changes in that folder.
Metadata changes (modify/access time, permissions, etc) are ignored.

Path comparisons are always case-sensitive, even on case-insensitive filesystems.
Use the exact on-disk casing for `<prefix>` and `<dbfile>`.

Regex patterns are case-sensitive by default; an explicit `(?i)` enables
case-insensitive pattern matching.

Always use slash (`/`) as the path separator in `<prefix>`, even on Windows.

Usage:

```
    FolderChecksum [OPTIONS] <rootdir> [<prefix>...]
```

For example:

Create a database file for this repo.

```
$ ./FolderChecksum -update ./
20:22:33 [INFO]  Using database file: .checksum.db
20:22:33 [INFO]  (worker 7) skipped: .checksum.db
new: .git/HEAD
new: .git/COMMIT_EDITMSG
...
new: worker_test.go
new: go.sum
20:22:34 [INFO]  stats: numFilesNew=176 numFilesChanged=0 numFilesDeleted=0 numFilesUnchanged=0 numVisitedFlagsCleared=176
```

Make some changes in this folder, then compare the current folder with
the database file created above.

```
$ ./FolderChecksum ./        
20:36:43 [INFO]  Using database file: .checksum.db
20:36:43 [INFO]  (worker 10) skipped: .checksum.db
changed: .git/ORIG_HEAD
changed: .git/index
...
deleted: worker.go
deleted: worker_test.go
20:36:43 [INFO]  stats: numFilesNew=0 numFilesChanged=10 numFilesDeleted=3 numFilesUnchanged=171 numVisitedFlagsCleared=0
```

Only scan the sub-folder `.git/logs`.

```
$ ./FolderChecksum ./ .git/logs
20:37:14 [INFO]  Using database file: .checksum.db
changed: .git/logs/HEAD
changed: .git/logs/refs/heads/main
20:37:14 [INFO]  stats: numFilesNew=0 numFilesChanged=2 numFilesDeleted=0 numFilesUnchanged=2 numVisitedFlagsCleared=0
```

Update the database with the current content of the folder.

```
$ ./FolderChecksum -update ./  
20:38:45 [INFO]  Using database file: .checksum.db
20:38:45 [INFO]  (worker 1) skipped: .checksum.db
changed: .git/index
changed: .git/logs/refs/heads/main
...
deleted: worker.go
deleted: worker_test.go
20:38:45 [INFO]  stats: numFilesNew=0 numFilesChanged=10 numFilesDeleted=3 numFilesUnchanged=171 numVisitedFlagsCleared=181
```

By default the database file `.checksum.db` is located under the specified
folder. So in the example above, the tool is using `.checksum.db` created
in this repo.

The list of new/changed/deleted files are written to `stdout`. The logs 
are written to `stderr`.

When `-update` is *not* used, this tool compares the content of the folder
with the database file, then outputs the list of new/changed/deleted files.

When `-update` is used, this tool outputs the list of new/changed/deleted
files and updates the database file with the current content of the folder.

By default this tool uses multiple threads to read the files. Please use
`-j 1` when scanning a folder on HDD.

# Checksum algorithm

Use `-checksum auto|sha1|md5` to select the checksum algorithm. The default
is `auto`:

- If `<dbfile>` contains saved checksums, use the length of the first
  nonempty checksum returned by the database: 32 hexadecimal characters
  means MD5, and 40 means SHA-1. Entries with NULL or empty checksums are
  skipped, and selection stops as soon as a nonempty checksum is found.
- If there are no saved checksums (including a database created with
  `-sizeonly`), or the first nonempty checksum has an unrecognized length,
  choose based on the OS, architecture, and CPU features that Go 1.20's
  implementation can use, as described below.
- Auto mode logs the selected algorithm and the reason at INFO level on
  `stderr`.

Manually specifying `-checksum sha1` or `-checksum md5` always uses that
algorithm. If saved checksums use a different algorithm, they are treated
as checksum mismatches and produce `changed` entries, even when the file
contents have not changed. With `-update`, the processed entries are saved
using the selected algorithm. Without `-update`, the saved checksums are
left unchanged.

Auto mode does not inspect the remaining checksums when selecting an
algorithm. In a database containing both MD5 and SHA-1 checksums, the
first nonempty checksum determines the algorithm; files saved with the
other algorithm produce `changed` entries when compared. If that first
checksum has an unrecognized length, use the same platform selection as
for a database with no saved checksums. The log includes the unrecognized
length and the reason for the chosen algorithm. Such saved checksums are
treated as mismatches and produce `changed` entries when compared.

`-sizeonly` ignores checksum and only compares file sizes.

Hashing uses Go's standard library. The table below describes normal
Go 1.20 builds for the six supported release targets:

| os | arch | md5 | sha1 |
| --- | --- | --- | --- |
| macOS (`darwin`) | `amd64` | Optimized scalar assembly | AVX2 when available; otherwise scalar assembly |
| macOS (`darwin`) | `arm64` | Optimized scalar assembly | Dedicated SHA-1 instructions on Apple Silicon |
| Linux | `amd64` | Optimized scalar assembly | AVX2 when available; otherwise scalar assembly |
| Linux | `arm64` | Optimized scalar assembly | Dedicated SHA-1 instructions when available; otherwise generic Go implementation |
| Windows | `amd64` | Optimized scalar assembly | AVX2 when available; otherwise scalar assembly |
| Windows | `arm64` | Optimized scalar assembly | Generic Go implementation, even if the CPU has SHA-1 instructions |

The SHA-1 AVX2 path requires AVX2, BMI1, and BMI2 CPU support; short inputs
and remaining blocks use scalar assembly. Go 1.20's MD5 implementation
does not use AVX2, and its SHA-1 implementation does not use the dedicated
x86 SHA extensions.

When the first nonempty checksum is absent or has an unrecognized length,
auto selection follows these implementations:

- On macOS, Linux, and Windows `amd64`, use SHA-1 when AVX2, BMI1, and BMI2
  are all available; otherwise use MD5. Dedicated x86 SHA instructions
  are not required and do not affect this choice.
- On macOS and Linux `arm64`, use SHA-1 when SHA-1 instructions are
  available to Go; otherwise use MD5.
- On Windows `arm64`, use MD5 because Go 1.20's SHA-1 implementation is
  generic, even on CPUs with SHA-1 instructions.

CPU features disabled through `GODEBUG` are treated as unavailable.
Auto mode logs the platform and the available implementation that led
to its choice. Recognized saved checksum lengths still take precedence.

# The database file

The schema of the database is simple. Each file has 4 columns -- `path`,
`size`, `checksum`, `visited`.

```
sqlite> select * from files where not path like ".git%";
path                   size   checksum                          visited
---------------------  -----  --------------------------------  -------
README.md              241    4d15b0cb8ec5a16e5ec8a33e8d0505b2  0      
go.sum                 177    b8196035843a5c84f5055fca95b27126  0      
fs_test.go             5713   8abf900b5a79a29085eaac71a5b93fba  0      
...
```

`path` is always separated by `'/'` (even on Windows), so the database
file generated on one platform can be used later on different platforms.
`visited` is used internally to detect deleted files. 

The database is always updated in a single transaction, i.e., updated
atomically in each invocation of the tool. Running multiple instances of
this tool on the same database file is **not** recommended (SQLite only
supports 1 concurrent write transaction anyway).

# Full usage

Note that part of the help message is generated using runtime information
(number of CPU cores, path separator `'/'` or `'\'`, etc.). To get the most
accurate help message on your system, please use `-h` by yourself.

```
Usage:

  FolderChecksum [OPTIONS] <rootdir> [<prefix>...]

  Calculate the checksums of <rootdir>'s subfiles, compare them against
  the checksums stored in <dbfile>, and update <dbfile> when -update is
  used.

  Path comparisons are always case-sensitive, even on case-insensitive
  filesystems. Use the exact on-disk casing for <prefix> and <dbfile>.

  Regex patterns are case-sensitive by default; an explicit (?i) enables
  case-insensitive pattern matching.

  Always use slash (/) as the path separator in <prefix>, even on Windows.

Positional Arguments:

  <rootdir>
    	The root folder to calculate the checksums. For each subfile, the
      path relative to <rootdir>, the size, and the checksum will be
    	stored into <dbfile>. <rootdir> must be a folder.

  <prefix>
    	Only process some of the files in <rootdir> whose relative path
    	starts with <prefix>. If multiple <prefix> are specified, the
    	user must make sure they are not overlapping with each other,
    	otherwise some assertions will be triggered. E.g., a/b and a/b/c
    	are overlapping, but a/b/c and a/b/d are not. Slash (/) should
    	always be used as the path separator in <prefix>, even on Windows.
    	For each specified <prefix>, the tool will perform:
    	  1. Clean the path to the shortest form.
    	  2. In the filesystem, recursively scan the entire subfolder if
    	     it's a folder, or scan the single file if it's a file. If it
    	     doesn't exist, go to step 3 directly.
    	  3. In the database, check the single entry '<prefix>' and all
    	     the entries that start with '<prefix>/'.

Options:

  -checksum string
        Choose auto, sha1, or md5. Auto uses the first nonempty saved
        checksum's length (32 for md5, 40 for sha1). If absent or unknown,
        choose sha1 on amd64 with AVX2 + BMI1 + BMI2, or supported arm64
        OSes with SHA-1 instructions. Otherwise use md5, including on
        Windows arm64 where Go 1.20 uses generic SHA-1.
        Auto logs the chosen algorithm and reason. If a manually
        specified algorithm differs from the saved checksums, they are
        treated as checksum mismatches and produce 'changed' entries.
        Ignored with -sizeonly.
         (default "auto")
  -dbfile string
    	Set database file name. If it doesn't contain any '/', the file
    	will be put into <rootdir> and will be automatically added to the
    	<exclude> list. If it contains at least one '/', the file will be
    	located using the path (for absolute paths) or current working
    	directory (for relative paths).
    	 (default ".checksum.db")
  -exclude value
    	Append a regex pattern to the <exclude> list. This option may be
    	repeated. See Pattern Matching section for more details.
  -followlinks
    	Follow symlinks as if the targets themselves are in the folder (
    	fail on broken links). By default symlinks in <rootdir> and <prefix>
    	are followed and others are skipped.
  -include value
    	Append a regex pattern to the <include> list. This option may be
    	repeated. See Pattern Matching section for more details.
  -j int
    	Set the number of workers to parallelly read the files. For SSD
    	only. Use 1 if <rootdir> is on a HDD.
    	 (default 16)
  -loglevel int
    	Set log level (ERROR=0, WARNING=1, INFO=2, DEBUG=3). Logs greater
    	than or equal to this level will be printed to stderr.
    	 (default 2)
  -sizeonly
    	Detect changes only by checking file sizes (instead of checksums).
  -update
    	Update the <dbfile>. By default this tool only compares current
    	<rootdir> against <dbfile> without modifying <dbfile>.
  -version
    	Display version number and exit.
    	
Pattern Matching:

  Use -exclude (or -include) to append a regex pattern to <exlude> (or
  <include>) list. Repeat them to add multiple patterns.

  The files that match any of the patterns in <exclude> list AND match
  none of the patterns in <include> list, will be excluded; otherwise
  they will be included. Excluded files will be treated as if they don't
  exist in the folder.

  The file paths relative to <rootdir> are matched against the patterns.
  For example, suppose <rootdir> is /path/to/dir which contains:
    /path/to/dir/file1
    /path/to/dir/subdir1/file1
    /path/to/dir/subdir2/
  Then these paths will be tested against the patterns:
    file1
    subdir1/file1
  Note that only files are tested (folders are ignored). Also note that
  slash (/) should always be used as the path separator in patterns, even
  on Windows.

  This tool will automatically add a leading '^' and trailing '$' for each
  specified pattern.
```

# Build

## Build native binary

SQLite requires CGO, you need to have a default C compiler on your system.

```sh
cd /path/to/FolderChecksum
go build
# FolderChecksum should appear.
```

## Cross-compile for amd64/arm64 darwin/linux/windows

SQLite requires CGO, so install the required C compilers first. The `CC_*`
environment variables below are optional overrides: leave them unset to use
the default compiler names from `PATH`, or set them to executable names or
paths to select different compilers. On non-macOS hosts, `CC_DARWIN_AMD64`
and `CC_DARWIN_ARM64` must be set to compilers with an Apple SDK.

```
  Target          Override variable       Default compiler
  darwin/amd64    CC_DARWIN_AMD64          clang (macOS host only)
  darwin/arm64    CC_DARWIN_ARM64          clang (macOS host only)
  linux/amd64     CC_LINUX_AMD64           x86_64-linux-musl-gcc
  linux/arm64     CC_LINUX_ARM64           aarch64-linux-musl-gcc
  windows/amd64   CC_WINDOWS_AMD64         x86_64-w64-mingw32-gcc
  windows/arm64   CC_WINDOWS_ARM64         aarch64-w64-mingw32-clang
```

Example to run `build-release-all.sh` on macOS (Apple Silicon), using Go 1.20
and extracted cross-compilers in `~/bin/toolchains/`:

```sh
cd /path/to/FolderChecksum

toolchains_dir="$HOME/bin/toolchains"

GO_BIN="/path/to/go1.20/bin/go" \
CC_LINUX_AMD64="$toolchains_dir/x86_64-unknown-linux-musl/bin/x86_64-linux-musl-gcc" \
CC_LINUX_ARM64="$toolchains_dir/aarch64-unknown-linux-musl/bin/aarch64-linux-musl-gcc" \
CC_WINDOWS_AMD64="$toolchains_dir/llvm-mingw-msvcrt/bin/x86_64-w64-mingw32-clang" \
CC_WINDOWS_ARM64="$toolchains_dir/llvm-mingw-20260922-ucrt-macos-universal/bin/aarch64-w64-mingw32-clang" \
./build-release-all.sh
```

The script creates six ZIP files in `dist/`, named
`FolderChecksum-{mac,linux,windows}-{amd64,arm64}.zip`.

### Details about the cross-compilers on macOS (Apple Silicon)

The macOS targets above use the native Apple Clang from Xcode Command Line
Tools, so `CC_DARWIN_AMD64` and `CC_DARWIN_ARM64` are not set.

The Linux targets above use the Apple Silicon musl toolchains from
[macOS cross-toolchains v15.2.0](https://github.com/messense/homebrew-macos-cross-toolchains/releases/tag/v15.2.0).

The Windows arm64 target above uses the macOS universal package from
[LLVM-MinGW 20260922](https://github.com/mstorsjo/llvm-mingw/releases/tag/20260922).

The Windows amd64 target uses `llvm-mingw-msvcrt` -- a separate copy
of LLVM-MinGW with its Windows amd64 C runtime rebuilt for MSVCRT and
Windows 7; it is not included in the UCRT download. The amd64 executable
uses Windows' built-in `msvcrt.dll`, so it does not require a UCRT
installation. Keep Go 1.20 for Windows 7 support.

## Run all unit tests

```sh
cd /path/to/FolderChecksum
go test .
```
