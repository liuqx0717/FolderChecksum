package main

import (
	"flag"
	"fmt"
	"os"
)

type flagValues []string

func (arr *flagValues) String() string {
	return fmt.Sprintf("%v", *arr)
}

func (arr *flagValues) Set(value string) error {
	*arr = append(*arr, value)
	return nil
}

type config struct {
	pathSep     string
	version     bool
	logLevel    int
	dbFile      string
	excludeList flagValues
	includeList flagValues
	followLinks bool
	update      bool
}

var cfg config

func init() {
	cfg.pathSep = string(os.PathSeparator)
	s := "'" + cfg.pathSep + "'"
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Usage:")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  FolderChecksum [OPTIONS] <rootdir> [<prefix>...]")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  Calculate the checksums of <rootdir>'s subfiles, compare them against")
		fmt.Fprintln(w, "  the checksums stored in <dbfile>, and update <dbfile> when -update is")
		fmt.Fprintln(w, "  used.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Positional Arguments:")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  <rootdir>")
		fmt.Fprintln(w, "    \tThe root folder to calculate the checksums. For each subfile, the")
		fmt.Fprintln(w, "    \tpath relative to <rootdir>, the size, and the md5 checksum will be")
		fmt.Fprintln(w, "    \tstored into <dbfile>.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  <prefix>")
		fmt.Fprintln(w, "    \tOnly process some of the files in <rootdir> whose relative path")
		fmt.Fprintln(w, "    \tstarts with <prefix>. If multiple <prefix> are specified, the")
		fmt.Fprintln(w, "    \tuser must make sure they are not overlapping with each other,")
		fmt.Fprintln(w, "    \totherwise some assertions will be triggered. E.g., a/b and a/b/c")
		fmt.Fprintln(w, "    \tare overlapping, but a/b/c and a/b/d are not. Slash (/) should")
		fmt.Fprintln(w, "    \talways be used as the path separator in <prefix>, even on Windows.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Options:")
		fmt.Fprintln(w, "")
		flag.PrintDefaults()
		fmt.Fprintln(w, "Pattern Matching:")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  Use -exclude (or -include) to append a regex pattern to <exlude> (or")
		fmt.Fprintln(w, "  <include>) list. Repeat them to add multiple patterns.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  The files that match any of the patterns in <exclude> list AND match")
		fmt.Fprintln(w, "  none of the patterns in <include> list, will be excluded; otherwise")
		fmt.Fprintln(w, "  they will be included. Excluded files will be treated as if they don't")
		fmt.Fprintln(w, "  exist in the folder.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  The file paths relative to <rootdir> are matched against the patterns.")
		fmt.Fprintln(w, "  For example, suppose <rootdir> is /path/to/dir which contains:")
		fmt.Fprintln(w, "    /path/to/dir/file1")
		fmt.Fprintln(w, "    /path/to/dir/subdir1/file1")
		fmt.Fprintln(w, "    /path/to/dir/subdir2/")
		fmt.Fprintln(w, "  Then these paths will be tested against the patterns:")
		fmt.Fprintln(w, "    file1")
		fmt.Fprintln(w, "    subdir1/file1")
		fmt.Fprintln(w, "  Note that only files are tested (folders are ignored). Also note that")
		fmt.Fprintln(w, "  slash (/) should always be used as the path separator in patterns, even")
		fmt.Fprintln(w, "  on Windows.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  This tool will automatically add a leading '^' and trailing '$' for each")
		fmt.Fprintln(w, "  specified pattern.")
		fmt.Fprintln(w, "")
	}
	flag.BoolVar(&cfg.version, "version", false,
		"Display version number and exit.\n")
	flag.IntVar(&cfg.logLevel, "loglevel", INFO,
		"Set log level (ERROR=0, WARNING=1, INFO=2, DEBUG=3). Logs greater\n"+
			"than or equal to this level will be printed to stderr.\n")
	flag.StringVar(&cfg.dbFile, "dbfile", ".checksum.db",
		"Set database file name. If it doesn't contain any "+s+", the file\n"+
			"will be put into <rootdir> and will be automatically added to the\n"+
			"<exclude> list. If it contains at least one "+s+", the file will be\n"+
			"located using the path (for absolute paths) or current working\n"+
			"directory (for relative paths).\n")
	flag.BoolVar(&cfg.followLinks, "followlinks", false,
		"Follow symlinks as if the targets themselves are in the folder.\n"+
			"By default symlinks are ignored.")
	flag.Var(&cfg.excludeList, "exclude",
		"Append a regex pattern to the <exclude> list. This option may be\n"+
			"repeated. See Pattern Matching section for more details.")
	flag.Var(&cfg.includeList, "include",
		"Append a regex pattern to the <include> list. This option may be\n"+
			"repeated. See Pattern Matching section for more details.")
	flag.BoolVar(&cfg.update, "update", false,
		"Update the <dbfile>. By default this tool only compares current\n"+
			"<rootdir> against <dbfile> without modifying <dbfile>.")
}
