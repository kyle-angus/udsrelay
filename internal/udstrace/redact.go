package udstrace

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Redact copies a trace log from r to w with identifying data removed, so hardware logs can be
// committed:
//
//   - Beacon payloads (tags beacons, beacon_netinfo, beacon_tag0/1, beacon_nodes) are emptied.
//     On hardware the game's first scan returns every nearby Wi-Fi access point, so these carry
//     third-party network names and MACs, as well as the consoles' own MACs and NodeInfo.
//   - netinfo payloads keep everything but the host MAC (bytes 0-5), which is zeroed.
//   - NodeInfo friend-code seeds are zeroed: words 2-3 of InitializeWithVersion requests and of
//     GetNodeInformation replies. The seed identifies a console's friend code.
//
// Changed lines gain redacted=<what>; everything else is kept. It returns how many lines changed.
func Redact(r io.Reader, w io.Writer) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	bw := bufio.NewWriter(w)
	n := 0
	for sc.Scan() {
		line := sc.Text()
		if out, ok := redactLine(line); ok {
			line = out
			n++
		}
		if _, err := bw.WriteString(line + "\n"); err != nil {
			return n, err
		}
	}
	if err := sc.Err(); err != nil {
		return n, err
	}
	return n, bw.Flush()
}

var (
	hexField   = regexp.MustCompile(` hex=([0-9a-fA-F]*)`)
	wordsField = regexp.MustCompile(` words=(\S*)`)
	tagField   = regexp.MustCompile(` tag=(\S+)`)
	cmdField   = regexp.MustCompile(` cmd=0x([0-9a-fA-F]{4})`)
)

func redactLine(line string) (string, bool) {
	if !strings.Contains(line, "UDSTRACE v=1 ") || !strings.Contains(line, "port="+PortUDS+" ") {
		return line, false
	}
	kind := field(line, "kind")
	switch kind {
	case "blob":
		tag := ""
		if m := tagField.FindStringSubmatch(line); m != nil {
			tag = m[1]
		}
		switch {
		case strings.HasPrefix(tag, "beacon"):
			return mark(hexField.ReplaceAllString(line, " hex="), "beacon"), true
		case tag == "netinfo":
			m := hexField.FindStringSubmatchIndex(line)
			if m == nil || m[3]-m[2] < 12 || line[m[2]:m[2]+12] == "000000000000" {
				return line, false // BeginHostingNetwork passes a zero MAC; nothing to hide
			}
			return mark(line[:m[2]]+"000000000000"+line[m[2]+12:], "mac"), true
		}
	case "req", "rsp":
		m := cmdField.FindStringSubmatch(line)
		if m == nil {
			return line, false
		}
		cmd := strings.ToLower(m[1])
		if (kind == "req" && cmd == "001b") || (kind == "rsp" && cmd == "000d") {
			return zeroWords(line, 2, 3)
		}
	}
	return line, false
}

// zeroWords sets the given indexes of the words= list to 0x00000000.
func zeroWords(line string, idx ...int) (string, bool) {
	m := wordsField.FindStringSubmatchIndex(line)
	if m == nil {
		return line, false
	}
	words := strings.Split(line[m[2]:m[3]], ",")
	changed := false
	for _, i := range idx {
		if i < len(words) && words[i] != "0x00000000" {
			words[i] = "0x00000000"
			changed = true
		}
	}
	if !changed {
		return line, false
	}
	return mark(line[:m[2]]+strings.Join(words, ",")+line[m[3]:], "seed"), true
}

// mark inserts redacted=<what> before the payload (hex= or words=) so it reads next to it.
func mark(line, what string) string {
	tag := fmt.Sprintf(" redacted=%s", what)
	for _, f := range []string{" hex=", " words="} {
		if i := strings.LastIndex(line, f); i >= 0 {
			return line[:i] + tag + line[i:]
		}
	}
	return line + tag
}

// field returns the value of a simple key=value field, or "".
func field(line, key string) string {
	i := strings.Index(line, " "+key+"=")
	if i < 0 {
		return ""
	}
	v := line[i+len(key)+2:]
	if j := strings.IndexByte(v, ' '); j >= 0 {
		v = v[:j]
	}
	return v
}
