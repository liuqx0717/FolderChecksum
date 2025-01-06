package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type flagValues []string

func (arr *flagValues) String() string {
	return fmt.Sprintf("%v", *arr)
}

func (arr *flagValues) Set(value string) error {
	*arr = append(*arr, value)
	return nil
}

type flags struct {
	version     bool
	logLevel    int
	j           int
	dbFile      string
	excludeList flagValues
	includeList flagValues
	followLinks bool
	sizeOnly    bool
	update      bool
	rootDir     string
	prefix      flagValues
}

var flg flags

type config struct {
	j           int
	dbFile      string
	db          *sql.DB        // thread safe
	excludeRe   *regexp.Regexp // thread safe
	includeRe   *regexp.Regexp // thread safe
	followLinks bool
	sizeOnly    bool
	update      bool
	outFile     io.Writer
	rootDir     string
	prefix      []string
}

func init() {
	s := "'" + string(os.PathSeparator) + "'"
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
		fmt.Fprintln(w, "    \tstored into <dbfile>. <rootdir> must be a folder.")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "  <prefix>")
		fmt.Fprintln(w, "    \tOnly process some of the files in <rootdir> whose relative path")
		fmt.Fprintln(w, "    \tstarts with <prefix>. If multiple <prefix> are specified, the")
		fmt.Fprintln(w, "    \tuser must make sure they are not overlapping with each other,")
		fmt.Fprintln(w, "    \totherwise some assertions will be triggered. E.g., a/b and a/b/c")
		fmt.Fprintln(w, "    \tare overlapping, but a/b/c and a/b/d are not. Slash (/) should")
		fmt.Fprintln(w, "    \talways be used as the path separator in <prefix>, even on Windows.")
		fmt.Fprintln(w, "    \tFor each specified <prefix>, the tool will perform:")
		fmt.Fprintln(w, "    \t  1. Clean the path to the shortest form.")
		fmt.Fprintln(w, "    \t  2. In the filesystem, recursively scan the entire subfolder if")
		fmt.Fprintln(w, "    \t     it's a folder, or scan the single file if it's a file. If it")
		fmt.Fprintln(w, "    \t     doesn't exist, go to step 3 directly.")
		fmt.Fprintln(w, "    \t  3. In the database, check the single entry '<prefix>' and all")
		fmt.Fprintln(w, "    \t     the entries that start with '<prefix>/'.")
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
	flag.BoolVar(&flg.version, "version", false,
		"Display version number and exit.\n")
	flag.IntVar(&flg.logLevel, "loglevel", INFO,
		"Set log level (ERROR=0, WARNING=1, INFO=2, DEBUG=3). Logs greater\n"+
			"than or equal to this level will be printed to stderr.\n")
	flag.IntVar(&flg.j, "j", runtime.NumCPU(),
		"Set the number of workers to parallelly read the files. For SSD\n"+
			"only. Use 1 if <rootdir> is on a HDD.\n")
	flag.StringVar(&flg.dbFile, "dbfile", ".checksum.db",
		"Set database file name. If it doesn't contain any "+s+", the file\n"+
			"will be put into <rootdir> and will be automatically added to the\n"+
			"<exclude> list. If it contains at least one "+s+", the file will be\n"+
			"located using the path (for absolute paths) or current working\n"+
			"directory (for relative paths).\n")
	flag.Var(&flg.excludeList, "exclude",
		"Append a regex pattern to the <exclude> list. This option may be\n"+
			"repeated. See Pattern Matching section for more details.")
	flag.Var(&flg.includeList, "include",
		"Append a regex pattern to the <include> list. This option may be\n"+
			"repeated. See Pattern Matching section for more details.")
	flag.BoolVar(&flg.followLinks, "followlinks", false,
		"Follow symlinks as if the targets themselves are in the folder (\n"+
			"fail on broken links). By default symlinks in <rootdir> and <prefix>\n"+
			"are followed and others are skipped.")
	flag.BoolVar(&flg.sizeOnly, "sizeonly", false,
		"Detect changes only by checking file sizes (instead of checksums).")
	flag.BoolVar(&flg.update, "update", false,
		"Update the <dbfile>. By default this tool only compares current\n"+
			"<rootdir> against <dbfile> without modifying <dbfile>.")
}

func parsePositionalArgs() {
	n := flag.NArg()
	if n < 1 {
		logFatal("Missing arg <rootDir>")
	}
	flg.rootDir = flag.Arg(0)
	for i := 1; i < n; i++ {
		flg.prefix = append(flg.prefix, flag.Arg(i))
	}
}

func getRegexFromList(patterns []string) *regexp.Regexp {
	regStr := "^"
	for _, pattern := range patterns {
		if _, err := regexp.Compile(pattern); err != nil {
			logFatal("Invalid regex pattern '%s': %s", pattern, err.Error())
		}
		regStr += "(" + pattern + ")|"
	}
	if regStr[len(regStr)-1] == '|' {
		regStr = regStr[:len(regStr)-1]
	}
	regStr += "$"

	logDebug("regStr: %s", regStr)
	return regexp.MustCompile(regStr)
}

func flagsToConfig(f *flags) *config {
	var cfg config

	containPathSep := strings.Contains(f.dbFile, string(os.PathSeparator))
	if !containPathSep && f.dbFile != path.Clean(f.dbFile) {
		logFatal("Cleaned dbFile '%s' not equal to original '%s'",
			path.Clean(f.dbFile), f.dbFile)
	}

	logLevel = f.logLevel
	if flg.j <= 0 {
		logFatal("j must >= 1")
	}
	cfg.j = flg.j

	if containPathSep {
		cfg.dbFile = f.dbFile
	} else {
		cfg.dbFile = filepath.Join(f.rootDir, f.dbFile)
	}
	cfg.dbFile = filepath.Clean(cfg.dbFile)
	cfg.db = mustOpenDb(cfg.dbFile)

	if containPathSep {
		cfg.excludeRe = getRegexFromList(f.excludeList)
	} else {
		cfg.excludeRe = getRegexFromList(
			append(f.excludeList,
				regexp.QuoteMeta(f.dbFile),
				regexp.QuoteMeta(f.dbFile+"-journal")))
	}

	cfg.includeRe = getRegexFromList(f.includeList)
	cfg.followLinks = f.followLinks
	cfg.sizeOnly = f.sizeOnly
	cfg.update = f.update
	cfg.outFile = os.Stdout
	cfg.rootDir = filepath.Clean(f.rootDir)

	for _, prefix := range f.prefix {
		cfg.prefix = append(cfg.prefix, cleanPrefix(prefix))
	}

	logDebug("flg: %+v", flg)
	logDebug("cfg: %+v", cfg)

	return &cfg
}
