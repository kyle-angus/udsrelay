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
//   - The relay's address in the shim's own notes ("relay mode: host:port", "checking
//     host:port", "cannot resolve host") becomes <relay>: it says where the relay runs.
//   - Every seed the log reveals (the two places above, and each node in DecryptBeaconData's
//     node list) is also zeroed wherever it appears in a payload: X/Y sends each console's seed
//     in one of its own data frames during a trade.
//
// Changed lines gain redacted=<what>; everything else is kept. It returns how many lines changed.
func Redact(r io.Reader, w io.Writer) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	seeds := collectSeeds(lines)
	bw := bufio.NewWriter(w)
	n := 0
	for _, line := range lines {
		out, ok := redactLine(line)
		if scrubbed, found := scrubSeeds(out, seeds); found {
			out, ok = scrubbed, true
		}
		if ok {
			line = out
			n++
		}
		if _, err := bw.WriteString(line + "\n"); err != nil {
			return n, err
		}
	}
	return n, bw.Flush()
}

// collectSeeds finds every friend-code seed the log shows, as the 16 hex digits of its
// little-endian bytes.
func collectSeeds(lines []string) map[string]bool {
	seeds := map[string]bool{}
	add := func(hexLE string) {
		if len(hexLE) == 16 && hexLE != "0000000000000000" {
			seeds[strings.ToLower(hexLE)] = true
		}
	}
	for _, line := range lines {
		if !strings.Contains(line, "UDSTRACE v=1 ") || !strings.Contains(line, "port="+PortUDS+" ") {
			continue
		}
		kind := field(line, "kind")
		m := cmdField.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cmd := strings.ToLower(m[1])
		switch {
		case (kind == "req" && cmd == "001b") || (kind == "rsp" && cmd == "000d"):
			wm := wordsField.FindStringSubmatch(line)
			if wm == nil {
				continue
			}
			words := strings.Split(wm[1], ",")
			if len(words) > 3 {
				add(wordLE(words[2]) + wordLE(words[3]))
			}
		case kind == "blob" && field(line, "tag") == "beacon_nodes":
			hm := hexField.FindStringSubmatch(line)
			if hm == nil {
				continue
			}
			const nodeHex = 2 * 0x28
			for i := 0; i+16 <= len(hm[1]); i += nodeHex {
				add(hm[1][i : i+16])
			}
		}
	}
	return seeds
}

// wordLE renders a 0x-prefixed word as the hex of its little-endian bytes.
func wordLE(w string) string {
	w = strings.TrimPrefix(strings.ToLower(w), "0x")
	if len(w) != 8 {
		return ""
	}
	return w[6:8] + w[4:6] + w[2:4] + w[0:2]
}

// scrubSeeds zeroes byte-aligned occurrences of any seed in the line's hex payload.
func scrubSeeds(line string, seeds map[string]bool) (string, bool) {
	m := hexField.FindStringSubmatchIndex(line)
	if m == nil || m[3] == m[2] || len(seeds) == 0 {
		return line, false
	}
	payload := []byte(strings.ToLower(line[m[2]:m[3]]))
	found := false
	for seed := range seeds {
		for from := 0; ; {
			i := strings.Index(string(payload[from:]), seed)
			if i < 0 {
				break
			}
			i += from
			if i%2 == 0 {
				copy(payload[i:], "0000000000000000")
				found = true
			}
			from = i + 1
		}
	}
	if !found {
		return line, false
	}
	out := line[:m[2]] + string(payload) + line[m[3]:]
	if !strings.Contains(out, " redacted=seed") {
		out = mark(out, "seed")
	}
	return out, true
}

var (
	hexField   = regexp.MustCompile(` hex=([0-9a-fA-F]*)`)
	relayAddr  = regexp.MustCompile(`(relay mode: |sockets ready; checking |cannot resolve )[^\s;]+`)
	wordsField = regexp.MustCompile(` words=(\S*)`)
	tagField   = regexp.MustCompile(` tag=(\S+)`)
	cmdField   = regexp.MustCompile(` cmd=0x([0-9a-fA-F]{4})`)
)

func redactLine(line string) (string, bool) {
	if !strings.Contains(line, "UDSTRACE v=1 ") {
		if strings.Contains(line, "Plugin <") && relayAddr.MatchString(line) {
			return relayAddr.ReplaceAllString(line, "${1}<relay>"), true
		}
		return line, false
	}
	if !strings.Contains(line, "port="+PortUDS+" ") {
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
