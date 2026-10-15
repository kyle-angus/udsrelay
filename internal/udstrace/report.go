package udstrace

import (
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Session is one parsed instance log.
type Session struct {
	Instance string
	Records  []Record
	Items    []Item
}

// NewSession parses and pairs one instance's records.
func NewSession(instance string, recs []Record) *Session {
	return &Session{Instance: instance, Records: recs, Items: Build(recs)}
}

// ShiftWall moves every timestamp in the session by shiftUs microseconds. Console clocks are
// not synchronised, so two consoles' logs need a manual offset to line up in one timeline.
func (s *Session) ShiftWall(shiftUs int64) {
	for i := range s.Records {
		if s.Records[i].Wall != 0 {
			s.Records[i].Wall = uint64(int64(s.Records[i].Wall) + shiftUs)
		}
	}
	for i := range s.Items {
		if s.Items[i].Wall != 0 {
			s.Items[i].Wall = uint64(int64(s.Items[i].Wall) + shiftUs)
		}
	}
}

// Calls returns the session's calls in order.
func (s *Session) Calls() []*Call {
	var out []*Call
	for _, it := range s.Items {
		if it.Call != nil {
			out = append(out, it.Call)
		}
	}
	return out
}

func startWall(sessions []*Session) uint64 {
	var start uint64
	for _, s := range sessions {
		for _, it := range s.Items {
			if it.Wall != 0 && (start == 0 || it.Wall < start) {
				start = it.Wall
			}
		}
	}
	return start
}

func rel(wall, start uint64) string {
	if wall < start {
		return "      ?"
	}
	return fmt.Sprintf("%+8.3fs", float64(wall-start)/1e6)
}

// quietKey returns a non-empty label when the item is an unchanged poll that the default
// timeline hides; prev tracks the last connection status shown per instance.
func quietKey(it Item, lastStatus map[string]string) string {
	if it.Rec != nil {
		if it.Rec.Kind == "signal" && strings.HasPrefix(it.Rec.Event, "NWM::BindNodeEvent") {
			return "data event"
		}
		if it.Rec.Kind == "note" && (it.Rec.Level == "Debug" || it.Rec.Level == "Trace" || it.Rec.Level == "Info") {
			return "debug note"
		}
		return ""
	}
	c := it.Call
	if c.Port != PortUDS {
		return ""
	}
	if code, ok := c.Result(); !ok || code != 0 {
		return ""
	}
	switch c.Cmd {
	case CmdGetConnectionStatus:
		s := ReplySummary(c)
		if lastStatus[c.Instance] == s {
			return "GetConnectionStatus"
		}
		lastStatus[c.Instance] = s
	case CmdPullPacket:
		if word(c.Reply, 2) == 0 {
			return "PullPacket empty"
		}
		return "PullPacket"
	case CmdSendTo:
		return "SendTo"
	case CmdGetChannel, CmdGetNodeInformation:
		return c.Name
	}
	return ""
}

// WriteTimeline writes the merged timeline. Unless verbose, unchanged polls, data traffic,
// per-packet events and debug notes are folded into one count line per instance.
func WriteTimeline(w io.Writer, sessions []*Session, verbose bool) {
	start := startWall(sessions)
	width := 0
	var lists [][]Item
	for _, s := range sessions {
		if len(s.Instance) > width {
			width = len(s.Instance)
		}
		lists = append(lists, s.Items)
	}
	hidden := map[string]map[string]int{}
	hiddenFrom := map[string]uint64{}
	hiddenTo := map[string]uint64{}
	lastStatus := map[string]string{}
	flush := func(inst string) {
		counts := hidden[inst]
		if len(counts) == 0 {
			return
		}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = fmt.Sprintf("%s x%d", k, counts[k])
		}
		fmt.Fprintf(w, "%s  %-*s    · %s (through %s)\n", rel(hiddenFrom[inst], start), width, inst,
			strings.Join(parts, ", "), strings.TrimSpace(rel(hiddenTo[inst], start)))
		delete(hidden, inst)
	}
	for _, it := range Merge(lists...) {
		if !verbose {
			if k := quietKey(it, lastStatus); k != "" {
				if hidden[it.Instance] == nil {
					hidden[it.Instance] = map[string]int{}
					hiddenFrom[it.Instance] = it.Wall
				}
				hidden[it.Instance][k]++
				hiddenTo[it.Instance] = it.Wall
				continue
			}
			flush(it.Instance)
		}
		fmt.Fprintf(w, "%s  %-*s  %s\n", rel(it.Wall, start), width, it.Instance, describeItem(it, verbose))
	}
	for _, s := range sessions {
		flush(s.Instance)
	}
}

func describeItem(it Item, verbose bool) string {
	if it.Call != nil {
		d := Describe(it.Call)
		if verbose {
			for _, b := range it.Call.Blobs {
				if b.Tag == "send" || b.Tag == "recv" {
					d += fmt.Sprintf(" [%s %s]", b.Tag, previewHex(b.Data, 16))
				}
			}
		}
		return d
	}
	r := it.Rec
	switch r.Kind {
	case "signal":
		return "signal " + r.Event
	case "note":
		return fmt.Sprintf("%s <%s> %s: %s", r.Class, r.Level, r.Func, r.Message)
	}
	return r.Kind
}

func previewHex(b []byte, n int) string {
	if len(b) <= n {
		return hex.EncodeToString(b)
	}
	return hex.EncodeToString(b[:n]) + "…"
}
