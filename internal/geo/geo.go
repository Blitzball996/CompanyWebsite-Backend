// Package geo provides offline IP-to-region lookup using the ip2region xdb (v2)
// data file. It implements a minimal, dependency-free reader of the well-known
// v2 xdb binary format (vector index + binary search over segment index).
//
// Region strings are formatted "国家|区域|省份|城市|ISP" where "0" means empty,
// e.g. "中国|0|广东省|深圳市|电信".
package geo

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
)

const (
	headerInfoLength      = 256
	vectorIndexRows       = 256
	vectorIndexCols       = 256
	vectorIndexSize       = 8
	segmentIndexSize      = 14
	vectorIndexLength     = vectorIndexRows * vectorIndexCols * vectorIndexSize
)

// Searcher holds the whole xdb file in memory for fast, lock-free lookups.
type Searcher struct {
	buf []byte
}

// Result is a parsed region.
type Result struct {
	Country string // 国家
	Region  string // 区域 (大区, 常为空)
	Province string // 省份
	City    string // 城市
	ISP     string // 运营商
}

var (
	defaultSearcher *Searcher
	once            sync.Once
)

// Open loads an xdb file into memory.
func Open(path string) (*Searcher, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(buf) < headerInfoLength+vectorIndexLength {
		return nil, errors.New("geo: xdb file too small or corrupt")
	}
	return &Searcher{buf: buf}, nil
}

// Init loads the package-level default searcher once. If it fails, lookups
// degrade gracefully to empty results instead of crashing the caller.
func Init(path string) error {
	var err error
	once.Do(func() {
		defaultSearcher, err = Open(path)
	})
	return err
}

// ipToUint32 parses a dotted IPv4 string to a big-endian uint32.
func ipToUint32(s string) (uint32, bool) {
	ip := net.ParseIP(s)
	if ip == nil {
		return 0, false
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0, false // IPv6 not supported by this v2 db
	}
	return binary.BigEndian.Uint32(v4), true
}

// isPrivate reports whether the IP is a private / loopback / link-local address.
func isPrivate(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// search returns the raw region string ("国家|区域|省份|城市|ISP") for an IP.
func (s *Searcher) search(ipStr string) (string, error) {
	ip, ok := ipToUint32(ipStr)
	if !ok {
		return "", errors.New("geo: not an IPv4 address")
	}

	// Locate the segment index range via the two-byte vector index.
	il0 := (ip >> 24) & 0xFF
	il1 := (ip >> 16) & 0xFF
	idx := il0*vectorIndexCols*vectorIndexSize + il1*vectorIndexSize
	pos := headerInfoLength + int(idx)
	if pos+8 > len(s.buf) {
		return "", errors.New("geo: vector index out of range")
	}
	startPtr := binary.LittleEndian.Uint32(s.buf[pos : pos+4])
	endPtr := binary.LittleEndian.Uint32(s.buf[pos+4 : pos+8])

	// Binary search over the segment index.
	low := startPtr
	high := endPtr
	for low <= high {
		mid := low + ((high-low)/segmentIndexSize/2)*segmentIndexSize
		off := int(mid)
		if off+segmentIndexSize > len(s.buf) {
			break
		}
		sip := binary.LittleEndian.Uint32(s.buf[off : off+4])
		eip := binary.LittleEndian.Uint32(s.buf[off+4 : off+8])
		switch {
		case ip < sip:
			if mid < segmentIndexSize {
				return "", errors.New("geo: not found")
			}
			high = mid - segmentIndexSize
		case ip > eip:
			low = mid + segmentIndexSize
		default:
			dataLen := binary.LittleEndian.Uint16(s.buf[off+8 : off+10])
			dataPtr := binary.LittleEndian.Uint32(s.buf[off+10 : off+14])
			d := int(dataPtr)
			if d+int(dataLen) > len(s.buf) {
				return "", errors.New("geo: data out of range")
			}
			return string(s.buf[d : d+int(dataLen)]), nil
		}
	}
	return "", errors.New("geo: not found")
}

// field normalizes ip2region's "0" placeholder to an empty string.
func field(s string) string {
	if s == "0" || s == "" {
		return ""
	}
	return s
}

// Lookup resolves an IP to a structured Result. It never errors out: unknown,
// private, or unparseable IPs return a zero/partial Result so callers can store
// whatever is available without failing the request.
func Lookup(ipStr string) Result {
	if isPrivate(ipStr) {
		return Result{Country: "内网", Province: "局域网"}
	}
	if defaultSearcher == nil {
		return Result{}
	}
	raw, err := defaultSearcher.search(ipStr)
	if err != nil {
		return Result{}
	}
	parts := strings.Split(raw, "|")
	r := Result{}
	if len(parts) > 0 {
		r.Country = field(parts[0])
	}
	if len(parts) > 1 {
		r.Region = field(parts[1])
	}
	if len(parts) > 2 {
		r.Province = field(parts[2])
	}
	if len(parts) > 3 {
		r.City = field(parts[3])
	}
	if len(parts) > 4 {
		r.ISP = field(parts[4])
	}
	return r
}
