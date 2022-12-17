package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/mackerelio/golib/pluginutil"
	"github.com/ryuichi1208/mackerel-plugin-axslog-light/axslog"

	"github.com/pkg/errors"
)

const (
	maxScanTokenSize = 1 * 1024 * 1024 // 1MiB
	startBufSize     = 4096
)

var f0 = float64(0)

// Reader struct
type Reader struct {
	Pos    int64
	reader io.Reader
}

// New :
func New(ir io.Reader, pos int64) (*Reader, error) {
	if is, ok := ir.(io.Seeker); ok {
		_, err := is.Seek(pos, 0)
		if err != nil {
			return nil, err
		}
	}
	return &Reader{
		Pos:    pos,
		reader: ir,
	}, nil
}

// Read :
func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.Pos += int64(n)
	return n, err
}

func parseLog(bs *bufio.Scanner, r axslog.Reader, opts axslog.CmdOpts) (float64, error) {
	filter := []byte(opts.Filter)
	for bs.Scan() {
		b := bs.Bytes()
		if len(filter) > 0 {
			if !bytes.Contains(b, filter) {
				continue
			}
		}
		_, pt1, pt2 := r.Parse(b)
		ptime1, err := axslog.BFloat64(pt1)
		if err != nil {
			log.Printf("Failed to convert ptime. continue: %v", err)
			continue
		}
		ptime2, err := axslog.BFloat64(pt2)
		if err != nil {
			log.Printf("Failed to convert ptime. continue: %v", err)
			continue
		}
		return (ptime1 - ptime2), nil
	}
	if bs.Err() != nil {
		return float64(0), bs.Err()
	}
	return float64(0), io.EOF
}

// parseFile :
func parseFile(logFile string, lastPos int64, opts axslog.CmdOpts, posFile string, stats *axslog.Stats) (float64, error) {
	maxReadSize := int64(0)
	maxReadSize = MaxReadSizeLTSV

	stat, err := os.Stat(logFile)
	if err != nil {
		return f0, errors.Wrap(err, "failed to stat log file")
	}

	fstat, err := axslog.FileStat(stat)
	if err != nil {
		return f0, errors.Wrap(err, "failed to inode of log file")
	}

	log.Printf("Analysis start logFile:%s lastPos:%d Size:%d", logFile, lastPos, stat.Size())

	if lastPos == 0 && stat.Size() > maxReadSize {
		// first time and big logile
		lastPos = stat.Size()
	}

	if stat.Size()-lastPos > maxReadSize {
		// big delay
		lastPos = stat.Size()
	}

	f, err := os.Open(logFile)
	if err != nil {
		return f0, errors.Wrap(err, "failed to open log file")
	}
	defer f.Close()
	fpr, err := New(f, lastPos)
	if err != nil {
		return f0, errors.Wrap(err, "failed to seek log file")
	}

	var ar axslog.Reader
	ar = NewLTSV(opts.RequestTime, opts.UpstreamTime)

	total := 0
	bs := bufio.NewScanner(fpr)
	bs.Buffer(make([]byte, startBufSize), maxScanTokenSize)
	for {
		ptime, errb := parseLog(bs, ar, opts)
		if errb == io.EOF {
			break
		}
		if errb != nil {
			return f0, errors.Wrap(errb, "Something wrong in parse log")
		}
		stats.Append(ptime)
		total++
	}

	log.Printf("Analysis completed logFile:%s startPos:%d endPos:%d Rows:%d", logFile, lastPos, fpr.Pos, total)

	// postionのupdate
	endTime := float64(0)
	if posFile != "" {
		endTime, err = axslog.WritePos(posFile, fpr.Pos, fstat)
		if err != nil {
			return endTime, errors.Wrap(err, "failed to update pos file")
		}
	}
	return endTime, nil
}

func getFileStats(opts axslog.CmdOpts, posFile, logFile string) (*axslog.Stats, error) {
	lastPos := int64(0)
	lastFstat := &axslog.FStat{}
	startTime := float64(0)
	endTime := float64(0)
	stats := axslog.NewStats()

	if axslog.FileExists(posFile) {
		l, s, f, err := axslog.ReadPos(posFile)
		if err != nil {
			return stats, errors.Wrap(err, "failed to load pos file")
		}
		lastPos = l
		startTime = s
		lastFstat = f
	}

	stat, err := os.Stat(logFile)
	if err != nil {
		return stats, errors.Wrap(err, "failed to stat log file")
	}
	fstat, err := axslog.FileStat(stat)
	if err != nil {
		return stats, errors.Wrap(err, "failed to get inode from log file")
	}
	if fstat.IsNotRotated(lastFstat) {
		endTime, err = parseFile(
			logFile,
			lastPos,
			opts,
			posFile,
			stats,
		)
		if err != nil {
			return stats, err
		}
	} else {
		// rotate!!
		log.Printf("Detect Rotate")
		lastFile, err := axslog.SearchFileByInode(filepath.Dir(logFile), lastFstat)
		if err != nil {
			log.Printf("Could not search previous file: %v", err)
			// new file
			endTime, err = parseFile(
				logFile,
				0, // lastPos
				opts,
				posFile,
				stats,
			)
			if err != nil {
				return stats, err
			}
		} else {
			// new file
			endTime, err = parseFile(
				logFile,
				0, // lastPos
				opts,
				posFile,
				stats,
			)
			if err != nil {
				return stats, err
			}
			// previous file
			_, err = parseFile(
				lastFile,
				lastPos,
				opts,
				"", // no update posfile
				stats,
			)
			if err != nil {
				log.Printf("Could not parse previous file: %v", err)
			}
		}
	}
	stats.SetDuration(endTime - startTime)
	return stats, nil
}

func getStats(opts axslog.CmdOpts) error {
	tmpDir := pluginutil.PluginWorkDir()
	curUser, _ := user.Current()
	uid := "0"
	if curUser != nil {
		uid = curUser.Uid
	}

	logfiles := strings.Split(opts.LogFile, ",")

	if len(logfiles) == 1 {
		posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s", uid, opts.KeyPrefix))
		stats, err := getFileStats(opts, posFile, opts.LogFile)
		if err != nil {
			return err
		}
		stats.Display(opts.KeyPrefix)
		return nil
	}

	sCh := make(chan axslog.StatsCh, len(logfiles))
	defer close(sCh)
	for _, l := range logfiles {
		logfile := l
		go func() {
			md5 := md5.Sum([]byte(logfile))
			posFile := filepath.Join(tmpDir, fmt.Sprintf("%s-axslog-v4-%s-%x", uid, opts.KeyPrefix, md5))
			stats, err := getFileStats(opts, posFile, logfile)
			sCh <- axslog.StatsCh{
				Stats:   stats,
				Logfile: logfile,
				Err:     err,
			}
		}()
	}
	errCnt := 0
	var statsAll []*axslog.Stats
	for range logfiles {
		s := <-sCh
		if s.Err != nil {
			errCnt++
			if len(logfiles) == errCnt {
				return s.Err
			}
			// warnings and ignore
			log.Printf("getStats file:%s :%v", s.Logfile, s.Err)
		} else {
			statsAll = append(statsAll, s.Stats)
		}
	}

	return nil
}

func _main() int {
	opts := axslog.CmdOpts{}
	psr := flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	_, err := psr.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	err = getStats(opts)
	if err != nil {
		log.Printf("getStats: %v", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(_main())
}
