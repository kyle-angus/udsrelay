package udstrace

import (
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// designTable lists the nwm::UDS commands the design doc's emulation table plans to implement,
// so the summary can flag commands a game uses that the plan does not cover.
var designTable = map[uint16]bool{
	CmdInitializeWithVersion: true, CmdShutdown: true, CmdInitializeDeprecated: true,
	CmdBeginHostingNetwork: true, CmdSetApplicationData: true, CmdGetApplicationData: true,
	CmdUpdateNetworkAttribute: true, CmdEjectClient: true, CmdEjectSpectators: true,
	CmdDestroyNetwork: true, CmdRecvBeaconBroadcastData: true, CmdConnectToNetwork: true,
	CmdDisconnectNetwork: true, CmdDecryptBeaconData: true, CmdBind: true, CmdUnbind: true,
	CmdSendTo: true, CmdPullPacket: true, CmdGetConnectionStatus: true,
	CmdGetNodeInformation: true, CmdGetChannel: true,
}

type cmdKey struct {
	Port string
	Cmd  uint16
}

type cmdStats struct {
	Name      string
	Count     map[string]int
	Failures  map[string]int
	FirstWall uint64
}

type flowKey struct {
	Instance, Dir string
	Chan          string
	Peer          string
	Flags         string
}

type flowStats struct {
	Count int
	Sizes []int
}

// WriteSummary writes the Markdown command-trace report that is Phase 1's exit artifact.
func WriteSummary(w io.Writer, sessions []*Session, title string) {
	start := startWall(sessions)
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("# UDS command trace: %s\n\n", title)
	if start != 0 {
		p("Captured %s UTC from %d instance logs with recon/azahar/udstrace.patch.\n\n",
			time.UnixMicro(int64(start)).UTC().Format("2006-01-02 15:04:05"), len(sessions))
	}

	// Sessions
	p("## Sessions\n\n| Instance | Duration | Calls | Processes calling UDS | Username |\n| --- | --- | --- | --- | --- |\n")
	for _, s := range sessions {
		var first, last uint64
		procs := map[string]bool{}
		user := ""
		calls := s.Calls()
		for _, c := range calls {
			if first == 0 {
				first = c.Req.Wall
			}
			last = c.Req.Wall
			if c.Proc != "" {
				procs[c.Proc] = true
			}
			if c.Port == PortUDS && (c.Cmd == CmdInitializeWithVersion || c.Cmd == CmdInitializeDeprecated) && user == "" {
				user = nodeUsername(c.Req.Words)
			}
		}
		p("| %s | %.1fs | %d | %s | %s |\n", s.Instance, float64(last-first)/1e6, len(calls), joinKeys(procs), user)
	}

	// Milestones
	p("\n## Milestones\n\n| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, s := range sessions {
		m := map[string]string{}
		mark := func(k string, c *Call) {
			if _, ok := m[k]; !ok {
				m[k] = strings.TrimSpace(rel(c.Req.Wall, start))
			}
		}
		for _, c := range s.Calls() {
			if c.Port != PortUDS {
				continue
			}
			code, ok := c.Result()
			success := ok && code == 0
			switch c.Cmd {
			case CmdInitializeWithVersion, CmdInitializeDeprecated:
				if success {
					mark("init", c)
				}
			case CmdBeginHostingNetwork, CmdBeginHostingDeprecated:
				if success {
					mark("host", c)
				}
			case CmdRecvBeaconBroadcastData:
				mark("scan", c)
			case CmdConnectToNetwork, CmdConnectDeprecated:
				if success {
					mark("join", c)
				}
			case CmdSendTo:
				if success {
					mark("send", c)
				}
			case CmdPullPacket:
				if success && word(c.Reply, 2) > 0 {
					mark("recv", c)
				}
			case CmdDisconnectNetwork, CmdDestroyNetwork, CmdShutdown:
				mark("leave", c)
			}
		}
		get := func(k string) string {
			if v, ok := m[k]; ok {
				return v
			}
			return "—"
		}
		p("| %s | %s | %s | %s | %s | %s | %s | %s |\n", s.Instance, get("init"), get("host"), get("scan"),
			get("join"), get("send"), get("recv"), get("leave"))
	}

	// Network
	p("\n## Network parameters\n\n")
	seenNet := false
	for _, s := range sessions {
		for _, c := range s.Calls() {
			nb := c.Blob("netinfo")
			if nb == nil {
				continue
			}
			ni, err := ParseNetworkInfo(nb.Data)
			if err != nil {
				continue
			}
			seenNet = true
			role := "hosted"
			if c.Cmd == CmdConnectToNetwork || c.Cmd == CmdConnectDeprecated {
				role = "joined (conn_type " + nb.Extra["conn_type"] + ")"
			}
			p("- **%s: %s via %s**: wlan comm ID `0x%08x`, ID8 `0x%02x`, max nodes %d, attributes `0x%04x`, network ID `0x%08x`, app data %d bytes",
				s.Instance, role, c.Name, ni.WlanCommID, ni.ID8, ni.MaxNodes, ni.Attributes, ni.NetworkID, ni.AppDataSize)
			if pb := c.Blob("passphrase"); pb != nil {
				p(", passphrase %d bytes `%s`", pb.Size, hex.EncodeToString(pb.Data))
			}
			p(".\n")
		}
	}
	for _, s := range sessions {
		for _, c := range s.Calls() {
			if b := c.Blob("appdata"); b != nil {
				p("- **%s: SetApplicationData** at %s: %d bytes `%s`\n", s.Instance,
					strings.TrimSpace(rel(c.Req.Wall, start)), b.Size, previewHex(b.Data, 32))
			}
		}
	}
	if !seenNet {
		p("No BeginHostingNetwork or ConnectToNetwork payloads were logged.\n")
	}

	// Commands
	stats := map[cmdKey]*cmdStats{}
	for _, s := range sessions {
		for _, c := range s.Calls() {
			k := cmdKey{c.Port, c.Cmd}
			st := stats[k]
			if st == nil {
				st = &cmdStats{Name: c.Name, Count: map[string]int{}, Failures: map[string]int{}, FirstWall: c.Req.Wall}
				stats[k] = st
			}
			st.Count[s.Instance]++
			if code, ok := c.Result(); c.Unimplemented || (ok && code != 0) {
				st.Failures[s.Instance]++
			}
			if c.Req.Wall < st.FirstWall {
				st.FirstWall = c.Req.Wall
			}
		}
	}
	keys := make([]cmdKey, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Port != keys[j].Port {
			return portOrder(keys[i].Port) < portOrder(keys[j].Port)
		}
		return keys[i].Cmd < keys[j].Cmd
	})
	p("\n## Commands used\n\nCounts per instance; failures (non-zero result or unimplemented) in brackets. \"Planned\" means the design doc's UDS emulation table covers it.\n\n")
	p("| Port | ID | Command |")
	for _, s := range sessions {
		p(" %s |", s.Instance)
	}
	p(" First seen | Planned |\n| --- | --- | --- |")
	for range sessions {
		p(" --- |")
	}
	p(" --- | --- |\n")
	var unplanned []string
	for _, k := range keys {
		st := stats[k]
		p("| %s | `0x%04X` | %s |", k.Port, k.Cmd, st.Name)
		for _, s := range sessions {
			n, f := st.Count[s.Instance], st.Failures[s.Instance]
			if f > 0 {
				p(" %d [%d] |", n, f)
			} else {
				p(" %d |", n)
			}
		}
		planned := "n/a"
		if k.Port == PortUDS {
			planned = "yes"
			if !designTable[k.Cmd] {
				planned = "**no**"
				unplanned = append(unplanned, fmt.Sprintf("%s (0x%04X)", st.Name, k.Cmd))
			}
		}
		p(" %s | %s |\n", strings.TrimSpace(rel(st.FirstWall, start)), planned)
	}
	if len(unplanned) > 0 {
		p("\nNot in the design table yet: %s.\n", strings.Join(unplanned, ", "))
	}

	// Failures
	p("\n## Failures and unimplemented commands\n\n")
	anyFail := false
	for _, s := range sessions {
		for _, c := range s.Calls() {
			code, ok := c.Result()
			if c.Unimplemented || (ok && code != 0) || (!ok && c.AwaitingWake) {
				anyFail = true
				p("- %s %s: %s\n", strings.TrimSpace(rel(c.Req.Wall, start)), s.Instance, Describe(c))
			}
		}
	}
	if !anyFail {
		p("None.\n")
	}

	// Events
	p("\n## Events signalled\n\n| Event |")
	for _, s := range sessions {
		p(" %s |", s.Instance)
	}
	p("\n| --- |")
	for range sessions {
		p(" --- |")
	}
	p("\n")
	events := map[string]map[string]int{}
	for _, s := range sessions {
		for _, r := range s.Records {
			if r.Kind == "signal" {
				if events[r.Event] == nil {
					events[r.Event] = map[string]int{}
				}
				events[r.Event][s.Instance]++
			}
		}
	}
	for _, e := range sortedKeys(events) {
		p("| %s |", e)
		for _, s := range sessions {
			p(" %d |", events[e][s.Instance])
		}
		p("\n")
	}

	// Binds and data
	p("\n## Data channels\n\n")
	for _, s := range sessions {
		for _, c := range s.Calls() {
			if c.Port == PortUDS && c.Cmd == CmdBind {
				p("- %s Bind: %s -> %s\n", s.Instance, Args(c), ReplySummary(c))
			}
		}
	}
	flows := map[flowKey]*flowStats{}
	for _, s := range sessions {
		for _, c := range s.Calls() {
			if c.Port != PortUDS {
				continue
			}
			var k flowKey
			var size int
			switch c.Cmd {
			case CmdSendTo:
				k = flowKey{s.Instance, "send", fmt.Sprint(word(c.Req.Words, 3) & 0xFF), nodeString(word(c.Req.Words, 2)),
					fmt.Sprintf("0x%02x", word(c.Req.Words, 6)&0xFF)}
				size = int(word(c.Req.Words, 5))
			case CmdPullPacket:
				if word(c.Reply, 2) == 0 {
					continue
				}
				k = flowKey{s.Instance, "recv", "bind " + fmt.Sprint(word(c.Req.Words, 1)), nodeString(word(c.Reply, 3)), "—"}
				size = int(word(c.Reply, 2))
			default:
				continue
			}
			f := flows[k]
			if f == nil {
				f = &flowStats{}
				flows[k] = f
			}
			f.Count++
			f.Sizes = append(f.Sizes, size)
		}
	}
	if len(flows) == 0 {
		p("\nNo data packets were sent or received.\n")
	} else {
		p("\n| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		fks := make([]flowKey, 0, len(flows))
		for k := range flows {
			fks = append(fks, k)
		}
		sort.Slice(fks, func(i, j int) bool {
			a, b := fks[i], fks[j]
			return fmt.Sprint(a) < fmt.Sprint(b)
		})
		for _, k := range fks {
			f := flows[k]
			sort.Ints(f.Sizes)
			p("| %s | %s | %s | %s | %s | %d | %d / %d / %d |\n", k.Instance, k.Dir, k.Chan, k.Peer, k.Flags, f.Count,
				f.Sizes[0], f.Sizes[len(f.Sizes)/2], f.Sizes[len(f.Sizes)-1])
		}
	}

	// NDM / AC
	p("\n## NDM and AC calls\n\n")
	anyOther := false
	for _, s := range sessions {
		for _, c := range s.Calls() {
			if c.Port == PortUDS {
				continue
			}
			anyOther = true
			p("- %s %s: %s\n", strings.TrimSpace(rel(c.Req.Wall, start)), s.Instance, Describe(c))
		}
	}
	if !anyOther {
		p("None logged. If ndm:u or ac:u should have been traced, check UDSTRACE_PORTS.\n")
	}
}

func portOrder(port string) int {
	switch port {
	case PortUDS:
		return 0
	case PortNDM:
		return 1
	}
	return 2
}

func joinKeys(m map[string]bool) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
