package axslog

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

type CmdOpts struct {
	LogFile      string `long:"logfile" description:"path to nginx ltsv logfiles. multiple log files can be specified, separated by commas." required:"true"`
	KeyPrefix    string `long:"key-prefix" description:"Metric key prefix" required:"true"`
	RequestTime  string `long:"request-time-key" default:"request_time" description:"key name for request_time"`
	UpstreamTime string `long:"upstream-time-key" default:"upstream_response_time" description:"key name for upstream_response_time"`
	Filter       string `long:"filter" default:"" description:"text for filtering log"`
}

// Reader :
type Reader interface {
	Parse([]byte) (int, []byte, []byte)
}

// Stats :
type Stats struct {
	f64s     sort.Float64Slice
	tf       float64
	total    float64
	duration float64
}

// StatsCh :
type StatsCh struct {
	Stats   *Stats
	Logfile string
	Err     error
}

// FilePos :
type FilePos struct {
	Pos   int64   `json:"pos"`
	Time  float64 `json:"time"`
	Inode uint64  `json:"inode"`
	Dev   uint64  `json:"dev"`
}

// FStat :
type FStat struct {
	Inode uint64
	Dev   uint64
}

// FileExists :
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// FileStat :
func FileStat(s os.FileInfo) (*FStat, error) {
	s2 := s.Sys().(*syscall.Stat_t)
	if s2 == nil {
		return &FStat{}, fmt.Errorf("Could not get Inode")
	}
	return &FStat{s2.Ino, uint64(s2.Dev)}, nil
}

// IsNotRotated :
func (fstat *FStat) IsNotRotated(lastFstat *FStat) bool {
	return lastFstat.Inode == 0 || lastFstat.Dev == 0 || (fstat.Inode == lastFstat.Inode && fstat.Dev == lastFstat.Dev)
}

// SearchFileByInode :
func SearchFileByInode(d string, fstat *FStat) (string, error) {
	files, err := ioutil.ReadDir(d)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		s, _ := FileStat(file)
		if s.Inode == fstat.Inode && s.Dev == fstat.Dev {
			return filepath.Join(d, file.Name()), nil
		}
	}
	return "", fmt.Errorf("There is no file by inode:%d in %s", fstat.Inode, d)
}

// WritePos :
func WritePos(filename string, pos int64, fstat *FStat) (float64, error) {
	now := float64(time.Now().Unix())
	fp := FilePos{pos, now, fstat.Inode, fstat.Dev}
	file, err := os.Create(filename)
	if err != nil {
		return now, err
	}
	defer file.Close()
	jb, err := json.Marshal(fp)
	if err != nil {
		return now, err
	}
	_, err = file.Write(jb)
	return now, err
}

// ReadPos :
func ReadPos(filename string) (int64, float64, *FStat, error) {
	fp := FilePos{}
	d, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0, 0, &FStat{}, err
	}
	err = json.Unmarshal(d, &fp)
	if err != nil {
		return 0, 0, &FStat{}, err
	}
	return fp.Pos, fp.Time, &FStat{fp.Inode, fp.Dev}, nil
}

func round(f float64) int64 {
	return int64(math.Round(f)) - 1
}

// NewStats :
func NewStats() *Stats {
	return &Stats{}
}

// GetTotal :
func (s *Stats) GetTotal() float64 {
	return s.total
}

// Append :
func (s *Stats) Append(ptime float64) {
	s.f64s = append(s.f64s, ptime)
	s.tf += ptime
}

// SetDuration :
func (s *Stats) SetDuration(d float64) {
	s.duration = d
}

// Display :
func (s *Stats) Display(keyPrefix string) {
	now := uint64(time.Now().Unix())
	sort.Sort(s.f64s)
	fl := float64(len(s.f64s))
	if len(s.f64s) > 0 {
		fmt.Printf("axslog.latency_%s.average\t%f\t%d\n", keyPrefix, s.tf/fl, now)
		fmt.Printf("axslog.latency_%s.99_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.99)], now)
		fmt.Printf("axslog.latency_%s.95_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.95)], now)
		fmt.Printf("axslog.latency_%s.90_percentile\t%f\t%d\n", keyPrefix, s.f64s[round(fl*0.90)], now)
	}
}

// BFloat64 :
func BFloat64(b []byte) (float64, error) {
	return strconv.ParseFloat(*(*string)(unsafe.Pointer(&b)), 64)
}
