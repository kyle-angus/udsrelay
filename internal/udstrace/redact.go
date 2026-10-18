package udstrace

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf16"
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
//   - nwm::UDS request words that carry an 8- or 16-bit argument are masked to the bits the
//     argument uses: callers leave stack leftovers in the rest, and the Download Play module's
//     leftovers include pieces of the console's friend-code seed (mk7-trace-hw-01).
//   - The relay's address in the shim's own notes ("relay mode: host:port", "checking
//     host:port", "cannot resolve host") becomes <relay>: it says where the relay runs.
//   - Every seed the log reveals (the two places above, and each node in DecryptBeaconData's
//     node list) is also zeroed wherever it appears in a payload: X/Y sends each console's seed
//     in one of its own data frames during a trade.
//   - Every username the log reveals (NodeInfo in InitializeWithVersion requests,
//     GetNodeInformation replies and DecryptBeaconData's node list) becomes a stand-in, P1, P2
//     and so on, in those words and wherever it appears in a payload as UTF-16: X/Y puts the
//     trainer's name in its data frames and application data, and the peer's is someone else's.
//
// Changed lines gain redacted=<what>; everything else is kept. It returns how many lines changed.
func Redact(r io.Reader, w io.Writer) (int, error) {
	n, err := RedactLogs([]io.Reader{r}, []io.Writer{w})
	return n[0], err
}

// RedactLogs redacts the logs of one capture together, ins[i] to outs[i], host first: a seed
// or username one console's log reveals is scrubbed from the other's too, and a console's
// username gets the same stand-in in both. It returns how many lines changed in each.
func RedactLogs(ins []io.Reader, outs []io.Writer) ([]int, error) {
	counts := make([]int, len(ins))
	logs := make([][]string, len(ins))
	for i, r := range ins {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			logs[i] = append(logs[i], sc.Text())
		}
		if err := sc.Err(); err != nil {
			return counts, err
		}
	}
	seeds := map[string]bool{}
	names := map[string]string{}
	for _, lines := range logs {
		for seed := range collectSeeds(lines) {
			seeds[seed] = true
		}
		collectNames(lines, names)
	}
	for i, lines := range logs {
		bw := bufio.NewWriter(outs[i])
		for _, line := range lines {
			out, ok := redactLine(line)
			if scrubbed, found := scrubSeeds(out, seeds); found {
				out, ok = scrubbed, true
			}
			if scrubbed, found := scrubNames(out, names); found {
				out, ok = scrubbed, true
			}
			if ok {
				line = out
				counts[i]++
			}
			if _, err := bw.WriteString(line + "\n"); err != nil {
				return counts, err
			}
		}
		if err := bw.Flush(); err != nil {
			return counts, err
		}
	}
	return counts, nil
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

// nodeInfoCall reports whether a nwm::UDS line carries a NodeInfo in words 2-11: an
// InitializeWithVersion request or a GetNodeInformation reply.
func nodeInfoCall(line string) bool {
	if !strings.Contains(line, "UDSTRACE v=1 ") || !strings.Contains(line, "port="+PortUDS+" ") {
		return false
	}
	m := cmdField.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	kind, cmd := field(line, "kind"), strings.ToLower(m[1])
	return (kind == "req" && cmd == "001b") || (kind == "rsp" && cmd == "000d")
}

// nodeInfoName returns the hex of a NodeInfo username field (20 bytes of UTF-16LE), cut at
// the first zero unit.
func nodeInfoName(b []byte) string {
	for i := 0; i+1 < len(b); i += 2 {
		if b[i] == 0 && b[i+1] == 0 {
			return hex.EncodeToString(b[:i])
		}
	}
	return hex.EncodeToString(b)
}

// collectNames adds every username the log shows to names (UTF-16LE hex of a username ->
// UTF-16LE hex of its stand-in), numbering new ones after those already known. Stand-ins
// (from redacting a redacted log) keep their own number.
func collectNames(lines []string, names map[string]string) {
	add := func(name string) {
		if name == "" || names[name] != "" {
			return
		}
		b, _ := hex.DecodeString(name)
		units := make([]uint16, len(b)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
		if standIn.MatchString(string(utf16.Decode(units))) {
			names[name] = name
			return
		}
		for n := len(names) + 1; ; n++ {
			s := utf16Hex(fmt.Sprintf("P%d", n))
			taken := false
			for _, v := range names {
				taken = taken || v == s
			}
			if !taken {
				names[name] = s
				return
			}
		}
	}
	for _, line := range lines {
		if nodeInfoCall(line) {
			if words := lineWords(line); len(words) >= 9 {
				add(nodeInfoName(wordsToBytes(words[4:9])))
			}
			continue
		}
		if strings.Contains(line, "UDSTRACE v=1 ") && field(line, "tag") == "beacon_nodes" {
			hm := hexField.FindStringSubmatch(line)
			if hm == nil {
				continue
			}
			b, _ := hex.DecodeString(hm[1])
			for i := 0; i+0x1c <= len(b); i += 0x28 {
				add(nodeInfoName(b[i+8 : i+0x1c]))
			}
		}
	}
}

// scrubNames swaps each known username for its stand-in: in the NodeInfo words of
// InitializeWithVersion and GetNodeInformation, and at byte-aligned UTF-16LE matches of two or
// more characters in the payload. A payload match keeps its length: the stand-in is
// zero-padded.
func scrubNames(line string, names map[string]string) (string, bool) {
	if len(names) == 0 {
		return line, false
	}
	changed := false
	if nodeInfoCall(line) {
		m := wordsField.FindStringSubmatchIndex(line)
		words := lineWords(line)
		if m != nil && len(words) >= 9 {
			nameBytes := wordsToBytes(words[4:9])
			name := nodeInfoName(nameBytes)
			if s := names[name]; s != "" && s != name {
				b, _ := hex.DecodeString(s)
				clear(nameBytes)
				copy(nameBytes, b)
				parts := strings.Split(line[m[2]:m[3]], ",")
				for i := 0; i < 5; i++ {
					parts[4+i] = fmt.Sprintf("0x%08x", binary.LittleEndian.Uint32(nameBytes[4*i:]))
				}
				line = line[:m[2]] + strings.Join(parts, ",") + line[m[3]:]
				changed = true
			}
		}
	}
	if m := hexField.FindStringSubmatchIndex(line); m != nil && m[3] > m[2] {
		payload := strings.ToLower(line[m[2]:m[3]])
		found := false
		for name, s := range names {
			if len(name) < 8 || s == name {
				continue // one character matches too much to mean anything
			}
			with := (s + strings.Repeat("0", len(name)))[:len(name)]
			for from := 0; ; {
				i := strings.Index(payload[from:], name)
				if i < 0 {
					break
				}
				i += from
				if i%2 == 0 {
					payload = payload[:i] + with + payload[i+len(name):]
					found = true
				}
				from = i + 1
			}
		}
		if found {
			line = line[:m[2]] + payload + line[m[3]:]
			changed = true
		}
	}
	if !changed {
		return line, false
	}
	if !strings.Contains(line, " redacted=name") {
		line = mark(line, "name")
	}
	return line, true
}

// lineWords parses a line's words= list; it returns nil if any word does not parse.
func lineWords(line string) []uint32 {
	m := wordsField.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	var words []uint32
	for _, w := range strings.Split(m[1], ",") {
		var v uint32
		if _, err := fmt.Sscanf(w, "0x%x", &v); err != nil {
			return nil
		}
		words = append(words, v)
	}
	return words
}

// utf16Hex renders s as the hex of its UTF-16LE encoding.
func utf16Hex(s string) string {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[2*i:], u)
	}
	return hex.EncodeToString(b)
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
	standIn    = regexp.MustCompile(`^P[0-9]+$`)
)

// narrowArgs lists, per nwm::UDS request, the words holding an 8- or 16-bit argument and the
// bits it uses (from the argument types Azahar's handlers read).
var narrowArgs = map[string]map[int]uint32{
	"0005": {1: 0xFFFF},                   // EjectClient(node)
	"0007": {1: 0xFFFF, 2: 0xFF},          // UpdateNetworkAttribute(bitmask, flag)
	"000d": {1: 0xFFFF},                   // GetNodeInformation(node)
	"0012": {3: 0xFF, 4: 0xFFFF},          // Bind(id, size, channel, node)
	"0017": {2: 0xFFFF, 3: 0xFF, 6: 0xFF}, // SendTo(unk, dest, channel, unk, size, flags)
	"001e": {1: 0xFF},                     // ConnectToNetwork(conn_type, ...)
}

// maskNarrowArgs clears the unused bits of a request's narrow arguments.
func maskNarrowArgs(line, cmd string) (string, bool) {
	masks := narrowArgs[cmd]
	m := wordsField.FindStringSubmatchIndex(line)
	if masks == nil || m == nil {
		return line, false
	}
	words := strings.Split(line[m[2]:m[3]], ",")
	changed := false
	for i, mask := range masks {
		if i >= len(words) {
			continue
		}
		var v uint32
		if _, err := fmt.Sscanf(words[i], "0x%x", &v); err != nil || v&^mask == 0 {
			continue
		}
		words[i] = fmt.Sprintf("0x%08x", v&mask)
		changed = true
	}
	if !changed {
		return line, false
	}
	return mark(line[:m[2]]+strings.Join(words, ",")+line[m[3]:], "padding"), true
}

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
		if kind == "req" {
			return maskNarrowArgs(line, cmd)
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
	if strings.Contains(line, tag+" ") {
		return line // redacting an already redacted log changes nothing
	}
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
