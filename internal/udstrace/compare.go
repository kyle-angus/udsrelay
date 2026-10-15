package udstrace

import (
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// sigCall is one significant call in protocol order.
type sigCall struct {
	Sig  string
	Wall uint64
}

// noisyCommands vary in count with timing and polling, so they are left out of the order diff.
var noisyCommands = map[uint16]bool{
	CmdGetConnectionStatus: true, CmdPullPacket: true, CmdSendTo: true, CmdGetChannel: true,
	CmdGetNodeInformation: true,
}

func callSig(c *Call) string {
	name := c.Name
	if c.Port != PortUDS {
		name = c.Port + " " + c.Name
	}
	code, ok := c.Result()
	switch {
	case c.Unimplemented:
		return name + " -> unimplemented"
	case !ok:
		return name + " -> no reply"
	case code == 0:
		return name
	default:
		return fmt.Sprintf("%s -> 0x%08X", name, code)
	}
}

// significantSequence lists a session's calls without polls and data traffic, with runs of
// the same call collapsed.
func significantSequence(s *Session) []sigCall {
	var out []sigCall
	for _, c := range s.Calls() {
		if c.Port == PortUDS && noisyCommands[c.Cmd] {
			continue
		}
		sig := callSig(c)
		if len(out) > 0 && out[len(out)-1].Sig == sig {
			continue
		}
		out = append(out, sigCall{Sig: sig, Wall: c.Req.Wall})
	}
	return out
}

type diffLine struct {
	Op  byte // ' ', '-', '+'
	Sig string
}

// lcsDiff returns an edit script turning a into b.
func lcsDiff(a, b []string) []diffLine {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, diffLine{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			out = append(out, diffLine{'-', a[i]})
			i++
		default:
			out = append(out, diffLine{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffLine{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffLine{'+', b[j]})
	}
	return out
}

type cmdUsage struct {
	Count   int
	Results map[string]bool
}

func usage(s *Session) map[string]*cmdUsage {
	u := map[string]*cmdUsage{}
	for _, c := range s.Calls() {
		name := c.Name
		if c.Port != PortUDS {
			name = c.Port + " " + c.Name
		}
		cu := u[name]
		if cu == nil {
			cu = &cmdUsage{Results: map[string]bool{}}
			u[name] = cu
		}
		cu.Count++
		switch code, ok := c.Result(); {
		case c.Unimplemented:
			cu.Results["unimplemented"] = true
		case !ok:
			cu.Results["no reply"] = true
		case code == 0:
			cu.Results["OK"] = true
		default:
			cu.Results[fmt.Sprintf("0x%08X", code)] = true
		}
	}
	return u
}

func resultList(cu *cmdUsage) string {
	if cu == nil {
		return "—"
	}
	rs := make([]string, 0, len(cu.Results))
	for r := range cu.Results {
		rs = append(rs, r)
	}
	sort.Strings(rs)
	return strings.Join(rs, ", ")
}

type netParams struct {
	Via        string
	CommID     uint32
	ID8        uint8
	MaxNodes   uint8
	AppData    int
	Passphrase string
}

func networkParams(s *Session) *netParams {
	for _, c := range s.Calls() {
		nb := c.Blob("netinfo")
		if nb == nil {
			continue
		}
		ni, err := ParseNetworkInfo(nb.Data)
		if err != nil {
			continue
		}
		p := &netParams{Via: c.Name, CommID: ni.WlanCommID, ID8: ni.ID8, MaxNodes: ni.MaxNodes, AppData: int(ni.AppDataSize)}
		if pb := c.Blob("passphrase"); pb != nil {
			p.Passphrase = hex.EncodeToString(pb.Data)
		}
		return p
	}
	return nil
}

type dataStats struct {
	Sent, Received   int
	MinSize, MaxSize int
	Channels         map[string]bool
}

func traffic(s *Session) dataStats {
	d := dataStats{MinSize: -1, Channels: map[string]bool{}}
	note := func(size int) {
		if d.MinSize < 0 || size < d.MinSize {
			d.MinSize = size
		}
		if size > d.MaxSize {
			d.MaxSize = size
		}
	}
	for _, c := range s.Calls() {
		if c.Port != PortUDS {
			continue
		}
		if code, ok := c.Result(); !ok || code != 0 {
			continue
		}
		switch c.Cmd {
		case CmdSendTo:
			d.Sent++
			d.Channels[fmt.Sprint(word(c.Req.Words, 3)&0xFF)] = true
			note(int(word(c.Req.Words, 5)))
		case CmdPullPacket:
			if n := int(word(c.Reply, 2)); n > 0 {
				d.Received++
				note(n)
			}
		}
	}
	return d
}

// CompareResult summarises how a candidate trace differs from the reference.
type CompareResult struct {
	OnlyInRef, OnlyInCand, ResultsDiffer []string
	OrderChanges                         int
	NetworkDiffers                       bool
}

// Matches reports whether the candidate uses the same commands with the same results, in the
// same order, on the same network parameters.
func (r CompareResult) Matches() bool {
	return len(r.OnlyInRef) == 0 && len(r.OnlyInCand) == 0 && len(r.ResultsDiffer) == 0 &&
		r.OrderChanges == 0 && !r.NetworkDiffers
}

// WriteCompare writes a Markdown comparison of a candidate trace against a reference trace
// for the same role (for example a console host log against an Azahar host log).
func WriteCompare(w io.Writer, ref, cand *Session) CompareResult {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	var res CompareResult

	ru, cu := usage(ref), usage(cand)
	names := map[string]bool{}
	for n := range ru {
		names[n] = true
	}
	for n := range cu {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var rows []string
	for _, n := range sorted {
		a, b := ru[n], cu[n]
		status := "same"
		switch {
		case a == nil:
			status = "**only in " + cand.Instance + "**"
			res.OnlyInCand = append(res.OnlyInCand, n)
		case b == nil:
			status = "**only in " + ref.Instance + "**"
			res.OnlyInRef = append(res.OnlyInRef, n)
		case resultList(a) != resultList(b):
			status = "**results differ**"
			res.ResultsDiffer = append(res.ResultsDiffer, n)
		}
		count := func(u *cmdUsage) int {
			if u == nil {
				return 0
			}
			return u.Count
		}
		rows = append(rows, fmt.Sprintf("| %s | %d | %d | %s | %s | %s |", n, count(a), count(b),
			resultList(a), resultList(b), status))
	}

	rs, cs := significantSequence(ref), significantSequence(cand)
	ra, ca := make([]string, len(rs)), make([]string, len(cs))
	for i := range rs {
		ra[i] = rs[i].Sig
	}
	for i := range cs {
		ca[i] = cs[i].Sig
	}
	diff := lcsDiff(ra, ca)
	for _, d := range diff {
		if d.Op != ' ' {
			res.OrderChanges++
		}
	}

	rn, cn := networkParams(ref), networkParams(cand)
	if (rn == nil) != (cn == nil) || (rn != nil && (rn.CommID != cn.CommID || rn.ID8 != cn.ID8 ||
		rn.MaxNodes != cn.MaxNodes || rn.Passphrase != cn.Passphrase)) {
		res.NetworkDiffers = true
	}

	verdict := "Match"
	if !res.Matches() {
		verdict = "Differs"
	}
	p("# Trace comparison: %s vs %s\n\n", cand.Instance, ref.Instance)
	p("**%s.** Reference: %s. Candidate: %s. ", verdict, ref.Instance, cand.Instance)
	p("%d command(s) only in the reference, %d only in the candidate, %d with different results, %d order change(s)",
		len(res.OnlyInRef), len(res.OnlyInCand), len(res.ResultsDiffer), res.OrderChanges)
	if res.NetworkDiffers {
		p(", network parameters differ")
	}
	p(".\n\n")

	p("## Commands\n\n| Command | %s calls | %s calls | %s results | %s results | Status |\n| --- | --- | --- | --- | --- | --- |\n",
		ref.Instance, cand.Instance, ref.Instance, cand.Instance)
	p("%s\n", strings.Join(rows, "\n"))

	p("\n## Call order\n\nSignificant calls in order, without polls (GetConnectionStatus, GetChannel, GetNodeInformation) and data (SendTo, PullPacket); repeats collapsed. `-` only in %s, `+` only in %s.\n\n```diff\n",
		ref.Instance, cand.Instance)
	for _, d := range diff {
		p("%c %s\n", d.Op, d.Sig)
	}
	p("```\n")

	p("\n## Network parameters\n\n")
	if rn == nil && cn == nil {
		p("Neither trace logged a network struct.\n")
	} else {
		p("| | %s | %s |\n| --- | --- | --- |\n", ref.Instance, cand.Instance)
		field := func(label string, get func(*netParams) string) {
			a, b := "—", "—"
			if rn != nil {
				a = get(rn)
			}
			if cn != nil {
				b = get(cn)
			}
			p("| %s | %s | %s |\n", label, a, b)
		}
		field("Via", func(n *netParams) string { return n.Via })
		field("wlan comm ID", func(n *netParams) string { return fmt.Sprintf("`0x%08x`", n.CommID) })
		field("ID8", func(n *netParams) string { return fmt.Sprintf("`0x%02x`", n.ID8) })
		field("Max nodes", func(n *netParams) string { return fmt.Sprint(n.MaxNodes) })
		field("App data bytes", func(n *netParams) string { return fmt.Sprint(n.AppData) })
		field("Passphrase", func(n *netParams) string { return "`" + n.Passphrase + "`" })
	}

	rt, ct := traffic(ref), traffic(cand)
	sizes := func(d dataStats) string {
		if d.MinSize < 0 {
			return "—"
		}
		return fmt.Sprintf("%d–%d", d.MinSize, d.MaxSize)
	}
	chans := func(d dataStats) string {
		ks := make([]string, 0, len(d.Channels))
		for k := range d.Channels {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		if len(ks) == 0 {
			return "—"
		}
		return strings.Join(ks, ", ")
	}
	p("\n## Data traffic\n\nCounts depend on how long each session ran, so they are context rather than part of the verdict.\n\n")
	p("| | %s | %s |\n| --- | --- | --- |\n", ref.Instance, cand.Instance)
	p("| Packets sent | %d | %d |\n| Packets received | %d | %d |\n| Payload bytes | %s | %s |\n| Send channels | %s | %s |\n",
		rt.Sent, ct.Sent, rt.Received, ct.Received, sizes(rt), sizes(ct), chans(rt), chans(ct))
	return res
}
