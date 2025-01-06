# Overview

This tool saves the current information of a folder to a database file.
Later the database file can be used to determine if anything has changed
in that folder. Metadata changes (modify/access time, permissions, etc)
are ignored.

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

Positional Arguments:

  <rootdir>
    	The root folder to calculate the checksums. For each subfile, the
    	path relative to <rootdir>, the size, and the md5 checksum will be
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
