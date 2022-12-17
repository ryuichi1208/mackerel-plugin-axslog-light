package main

import (
	"bytes"
)

// MaxReadSizeLTSV : Maximum size for read
var MaxReadSizeLTSV int64 = 1000 * 1000 * 1000

// Reader struct
type ReaderLTSV struct {
	bytePtimeKey1 []byte
	bytePtimeKey2 []byte
}

// New :
func NewLTSV(ptimeKey1, ptimeKey2 string) *ReaderLTSV {
	return &ReaderLTSV{[]byte(ptimeKey1), []byte(ptimeKey2)}
}

var bTab = []byte("\t")
var bCol = []byte(":")
var bHif = []byte("-")

// Parse :
func (r *ReaderLTSV) Parse(d1 []byte) (int, []byte, []byte) {
	c := 1
	var pt1, pt2 []byte
	p1 := 0
	dlen := len(d1)
	for {
		if dlen == p1 {
			break
		}
		p2 := bytes.Index(d1[p1:], bTab)
		if p2 < 0 {
			p2 = dlen - p1 - 1
		}
		p3 := bytes.Index(d1[p1:p1+p2], bCol)
		if p3 < 0 {
			break
		}

		// `-` はskip
		if bytes.Equal(d1[p1+p3+1:p1+p2], bHif) {
			p1 += p2 + 1
			continue
		}

		if bytes.Equal(d1[p1:p1+p3], r.bytePtimeKey1) {
			pt1 = d1[p1+p3+1 : p1+p2]
			p1 += p2 + 1
			continue
		}

		if bytes.Equal(d1[p1:p1+p3], r.bytePtimeKey2) {
			pt2 = d1[p1+p3+1 : p1+p2]
		}

		p1 += p2 + 1
	}
	if len(pt2) == 0 {
		pt2 = []byte("0.0000")
	}
	return c, pt1, pt2
}
