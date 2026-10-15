// Package udstrace parses Azahar logs produced with recon/azahar/udstrace.patch and turns
// them into paired, decoded IPC calls for the Phase 1 command trace.
package udstrace

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Record is one log line of interest: a UDSTRACE line or a stock NWM/NDM log line.
type Record struct {
	Instance string `json:"instance"`
	Line     int    `json:"line"`
	// Kind is req, rsp, unimpl, wake, signal, blob or note.
	Kind     string   `json:"kind"`
	Wall     uint64   `json:"wall_us"`
	Port     string   `json:"port,omitempty"`
	Cmd      uint16   `json:"cmd,omitempty"`
	Name     string   `json:"name,omitempty"`
	Tid      uint32   `json:"tid,omitempty"`
	Proc     string   `json:"proc,omitempty"`
	Hdr      uint32   `json:"hdr,omitempty"`
	Words    []uint32 `json:"words,omitempty"`
	Deferred bool     `json:"deferred,omitempty"`
	// Sess identifies the shim session a req/rsp/blob belongs to; empty in Azahar logs, whose
	// replies always follow their request.
	Sess  string            `json:"sess,omitempty"`
	Event string            `json:"event,omitempty"`
	Tag   string            `json:"tag,omitempty"`
	Size  int               `json:"size,omitempty"`
	Data  []byte            `json:"data,omitempty"`
	Extra map[string]string `json:"extra,omitempty"`
	// Note fields, for stock log lines.
	Level   string `json:"level,omitempty"`
	Class   string `json:"class,omitempty"`
	Func    string `json:"func,omitempty"`
	Message string `json:"message,omitempty"`
}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	// [  12.345678] Service.NWM <Debug> core/hle/service/nwm/nwm_uds.cpp:BeginHostingNetwork:1011: called
	noteRe = regexp.MustCompile(`^\[\s*\d+\.\d+\] (\S+) <(\w+)> [^:]+:(\w+):\d+: (.*)$`)
)

// noteClasses are the stock log classes kept as notes alongside the trace.
var noteClasses = map[string]bool{
	"Service.NWM": true,
	"Service.NDM": true,
	"Service.AC":  true,
	"Network":     true,
	"Plugin":      true, // the console shim's own diagnostics
}

const tracePrefix = "UDSTRACE v=1 "

// Parse reads one Azahar log. Notes get the wall time of the latest UDSTRACE line before them.
func Parse(r io.Reader, instance string) ([]Record, error) {
	var out []Record
	var lastWall uint64
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := ansiRe.ReplaceAllString(sc.Text(), "")
		if i := strings.Index(line, tracePrefix); i >= 0 {
			rec, err := parseTrace(line[i+len(tracePrefix):])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", instance, lineNo, err)
			}
			rec.Instance, rec.Line = instance, lineNo
			lastWall = rec.Wall
			out = append(out, rec)
			continue
		}
		m := noteRe.FindStringSubmatch(line)
		if m == nil || !noteClasses[m[1]] {
			continue
		}
		out = append(out, Record{
			Instance: instance, Line: lineNo, Kind: "note", Wall: lastWall,
			Class: m[1], Level: m[2], Func: m[3], Message: m[4],
		})
	}
	return out, sc.Err()
}

// splitFields splits `a=1 b="x y" c=3` into a map, honouring double quotes.
func splitFields(s string) (map[string]string, error) {
	fields := map[string]string{}
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ")
		if s == "" {
			break
		}
		eq := strings.IndexByte(s, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("malformed field near %q", s)
		}
		key := s[:eq]
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				return nil, fmt.Errorf("unterminated quote for %s", key)
			}
			val, s = s[1:end+1], s[end+2:]
		} else if sp := strings.IndexByte(s, ' '); sp >= 0 {
			val, s = s[:sp], s[sp:]
		} else {
			val, s = s, ""
		}
		fields[key] = val
	}
	return fields, nil
}

func parseUint(s string, bits int) (uint64, error) {
	return strconv.ParseUint(s, 0, bits)
}

func parseTrace(body string) (Record, error) {
	f, err := splitFields(body)
	if err != nil {
		return Record{}, err
	}
	rec := Record{Kind: f["kind"], Port: f["port"], Name: f["name"], Proc: f["proc"],
		Event: f["event"], Tag: f["tag"]}
	if rec.Wall, err = parseUint(f["wall"], 64); err != nil {
		return rec, fmt.Errorf("wall: %w", err)
	}
	if v, ok := f["cmd"]; ok {
		n, err := parseUint(v, 16)
		if err != nil {
			return rec, fmt.Errorf("cmd: %w", err)
		}
		rec.Cmd = uint16(n)
	}
	if v, ok := f["hdr"]; ok {
		n, err := parseUint(v, 32)
		if err != nil {
			return rec, fmt.Errorf("hdr: %w", err)
		}
		rec.Hdr = uint32(n)
	}
	if v, ok := f["tid"]; ok {
		n, _ := parseUint(v, 32)
		rec.Tid = uint32(n)
	}
	if v := f["words"]; v != "" {
		for _, w := range strings.Split(v, ",") {
			n, err := parseUint(w, 32)
			if err != nil {
				return rec, fmt.Errorf("words: %w", err)
			}
			rec.Words = append(rec.Words, uint32(n))
		}
	}
	rec.Sess = f["sess"]
	rec.Deferred = f["deferred"] == "1"
	if v, ok := f["size"]; ok {
		n, _ := strconv.Atoi(v)
		rec.Size = n
	}
	if v, ok := f["hex"]; ok {
		if rec.Data, err = hex.DecodeString(v); err != nil {
			return rec, fmt.Errorf("hex: %w", err)
		}
	}
	for _, k := range []string{"dest", "chan", "flags", "bind", "src", "conn_type"} {
		if v, ok := f[k]; ok {
			if rec.Extra == nil {
				rec.Extra = map[string]string{}
			}
			rec.Extra[k] = v
		}
	}
	switch rec.Kind {
	case "req", "rsp", "unimpl", "wake", "signal", "blob":
	default:
		return rec, fmt.Errorf("unknown kind %q", rec.Kind)
	}
	return rec, nil
}
