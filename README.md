# Overview

This tool saves the current information of a folder to a database file.
Later the database file can be used to determine if anything has changed
in that folder. Metadata changes (modify/access time, permissions, etc)
are ignored.

For example:

```
# Create a database file for this repo

$ ./FolderChecksum -update ./
20:22:33 [INFO]  Using database file: .checksum.db
20:22:33 [INFO]  (worker 7) skipped: .checksum.db
new: .git/HEAD
new: .git/COMMIT_EDITMSG
...
new: worker_test.go
new: go.sum
20:22:34 [INFO]  stats: numFilesNew=176 numFilesChanged=0 numFilesDeleted=0 numFilesUnchanged=0 numVisitedFlagsCleared=176

# Make some changes in this folder, then compare the current folder with
# the previously created database file.
$ ./FolderChecksum -update ./
```

By default the database file `.checksum.db` is located under the specified
folder. So in the example above, the tool is using `.checksum.db` created
in this repo.

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

`visited` is used internally to detect deleted files.