package udstrace

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) *Session {
	t.Helper()
	f, err := os.Open("testdata/" + name + ".log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := Parse(f, name)
	if err != nil {
		t.Fatal(err)
	}
	return NewSession(name, recs)
}

func findCall(t *testing.T, s *Session, port string, cmd uint16) *Call {
	t.Helper()
	for _, c := range s.Calls() {
		if c.Port == port && c.Cmd == cmd {
			return c
		}
	}
	t.Fatalf("%s: no call %s 0x%04X", s.Instance, port, cmd)
	return nil
}

func TestParseTraceLine(t *testing.T) {
	line := "\x1b[0;36m[  12.000345] Service <Info> core/hle/service/service.cpp:HandleSyncRequest:192: " +
		`UDSTRACE v=1 kind=req wall=1792000000000000 port=nwm::UDS cmd=0x0017 name="SendTo" tid=33 ` +
		`proc="my game" hdr=0x00170182 words=0x00170182,0x00000000,0x00000002` + "\x1b[0m"
	recs, err := Parse(strings.NewReader(line), "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records", len(recs))
	}
	r := recs[0]
	if r.Kind != "req" || r.Cmd != 0x17 || r.Name != "SendTo" || r.Proc != "my game" || r.Hdr != 0x00170182 {
		t.Errorf("bad record: %+v", r)
	}
	if len(r.Words) != 3 || r.Words[2] != 2 {
		t.Errorf("words = %v", r.Words)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	for _, line := range []string{
		"UDSTRACE v=1 kind=req wall=notanumber port=nwm::UDS",
		"UDSTRACE v=1 kind=bogus wall=1",
		`UDSTRACE v=1 kind=signal wall=1 event="unterminated`,
		"UDSTRACE v=1 kind=blob wall=1 tag=send hex=zz",
	} {
		if _, err := Parse(strings.NewReader(line), "a"); err == nil {
			t.Errorf("expected error for %q", line)
		}
	}
}

func TestNotesKeepOnlyNetworkClasses(t *testing.T) {
	log := "[   1.000000] Service.NWM <Debug> core/hle/service/nwm/nwm_uds.cpp:Shutdown:618: called\n" +
		"[   1.000000] Render.OpenGL <Info> video_core/x.cpp:Foo:1: not ours\n"
	recs, err := Parse(strings.NewReader(log), "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Func != "Shutdown" || recs[0].Message != "called" {
		t.Errorf("records = %+v", recs)
	}
}

func TestHostSession(t *testing.T) {
	s := loadFixture(t, "host")

	host := findCall(t, s, PortUDS, CmdBeginHostingNetwork)
	ni, err := ParseNetworkInfo(host.Blob("netinfo").Data)
	if err != nil {
		t.Fatal(err)
	}
	if ni.WlanCommID != 0x00123456 || ni.ID8 != 2 || ni.MaxNodes != 2 || ni.AppDataSize != 32 {
		t.Errorf("network info = %+v", ni)
	}
	if got := host.Blob("passphrase").Data; string(got) != "FIXTURE-PASSPHRASE-NOT-REAL" {
		t.Errorf("passphrase = %q", got)
	}

	init := findCall(t, s, PortUDS, CmdInitializeWithVersion)
	if u := nodeUsername(init.Req.Words); u != "Serena" {
		t.Errorf("username = %q", u)
	}

	unimpl := findCall(t, s, PortUDS, 0x15)
	if !unimpl.Unimplemented || ReplySummary(unimpl) != "UNIMPLEMENTED in Azahar" {
		t.Errorf("SetMaxSendDelay not marked unimplemented: %+v", unimpl)
	}

	var statuses []string
	for _, c := range s.Calls() {
		if c.Cmd == CmdGetConnectionStatus {
			cs, ok := ParseConnectionStatus(c.Reply)
			if !ok {
				t.Fatal("short connection status")
			}
			statuses = append(statuses, cs.String())
		}
	}
	want := "ConnectedAsHost reason=ConnectionEstablished node=1 nodes=2/2 bitmask=0x0003 changed=0x0002"
	if statuses[len(statuses)-1] != want {
		t.Errorf("last status = %q, want %q", statuses[len(statuses)-1], want)
	}
}

func TestClientDeferredConnect(t *testing.T) {
	s := loadFixture(t, "client")

	conn := findCall(t, s, PortUDS, CmdConnectToNetwork)
	if !conn.AwaitingWake || conn.Wake == nil {
		t.Fatalf("ConnectToNetwork not paired with its wake-up: %+v", conn)
	}
	if code, ok := conn.Result(); !ok || code != 0 {
		t.Errorf("ConnectToNetwork result = %#x, %v", code, ok)
	}
	if conn.Blob("netinfo").Extra["conn_type"] != "1" {
		t.Errorf("conn_type missing")
	}

	// RecvBeaconBroadcastData writes its reply before sleeping, so deferred=1 still has a reply.
	scan := findCall(t, s, PortUDS, CmdRecvBeaconBroadcastData)
	if scan.AwaitingWake {
		t.Error("scan should not await a wake-up: its reply header differs from the request")
	}
	if !strings.Contains(Args(scan), "comm_id=0x00123456") {
		t.Errorf("scan args = %s", Args(scan))
	}

	pull := 0
	for _, c := range s.Calls() {
		if c.Cmd == CmdPullPacket && c.Blob("recv") != nil {
			pull++
		}
	}
	if pull != 4 {
		t.Errorf("received %d packets, want 4", pull)
	}
}

// A reply whose header matches the request is only a pending wake-up when it is marked deferred.
// idle-01 showed ndm:u OverrideDefaultDaemons (one word in, one out) reported as "(no wake-up logged)".
func TestSameShapeReplyNotDeferred(t *testing.T) {
	log := `[ 1.0] Service <Info> x: UDSTRACE v=1 kind=req wall=1000 port=ndm:u cmd=0x0014 name="OverrideDefaultDaemons" tid=1 proc="kujira-1" hdr=0x00140040 words=0x00140040,0x0000000f
[ 1.0] Service <Info> x: UDSTRACE v=1 kind=rsp wall=1001 port=ndm:u cmd=0x0014 hdr=0x00140040 deferred=0 words=0x00140040,0x00000000
`
	recs, err := Parse(strings.NewReader(log), "host")
	if err != nil {
		t.Fatal(err)
	}
	c := findCall(t, NewSession("host", recs), "ndm:u", 0x14)
	if c.AwaitingWake {
		t.Error("deferred=0 reply treated as awaiting a wake-up")
	}
	if code, ok := c.Result(); !ok || code != 0 {
		t.Errorf("result = %#x, %v; want OK", code, ok)
	}
}

// The console shim forwards X/Y's two nwm::UDS sessions at once, so a SendTo on one and a
// PullPacket on the other interleave. Each reply must reach its own request.
func TestInterleavedSessions(t *testing.T) {
	const pfx = "[ 1.0] Service <Info> shim:Forward:0: UDSTRACE v=1 "
	lines := []string{
		`kind=req wall=1000 port=nwm::UDS sess=1 cmd=0x0017 name="SendTo" tid=0 proc="p" hdr=0x00170182 words=0x00170182,0x00000002,0x00000002,0x000000f3,0x00000000,0x00000040,0x00000002,0x00004002,0x08000000`,
		`kind=req wall=1001 port=nwm::UDS sess=2 cmd=0x0014 name="PullPacket" tid=0 proc="p" hdr=0x001400c0 words=0x001400c0,0x00000002,0x000005dc,0x00000000`,
		`kind=rsp wall=1002 port=nwm::UDS sess=1 cmd=0x0017 hdr=0x00170040 deferred=0 words=0x00170040,0x00000000`,
		`kind=rsp wall=1003 port=nwm::UDS sess=2 cmd=0x0014 hdr=0x001400c2 deferred=0 words=0x001400c2,0x00000000,0x00000000,0x00000000,0x00000002,0x08000000`,
		// Old logs without sess still pair by port and command.
		`kind=req wall=1004 port=nwm::UDS cmd=0x000a name="DisconnectNetwork" tid=0 proc="p" hdr=0x000a0000 words=0x000a0000`,
		`kind=req wall=1005 port=nwm::UDS cmd=0x0014 name="PullPacket" tid=0 proc="p" hdr=0x001400c0 words=0x001400c0,0x00000002,0x000005dc,0x00000000`,
		`kind=rsp wall=1006 port=nwm::UDS cmd=0x000a hdr=0x000a0040 deferred=0 words=0x000a0040,0x00000000`,
	}
	var log strings.Builder
	for _, l := range lines {
		log.WriteString(pfx + l + "\n")
	}
	recs, err := Parse(strings.NewReader(log.String()), "console")
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession("console", recs)
	for _, cmd := range []uint16{0x17, 0x14, 0x0a} {
		c := findCall(t, s, PortUDS, cmd)
		if code, ok := c.Result(); !ok || code != 0 {
			t.Errorf("%s: result = %#x, %v; want OK", c.Name, code, ok)
		}
	}
	if s.Calls()[0].Req.Sess != "1" {
		t.Errorf("sess not parsed: %q", s.Calls()[0].Req.Sess)
	}
}

func TestRedact(t *testing.T) {
	const pfx = "[ 1.0] Service <Info> shim:TraceBlob:0: UDSTRACE v=1 "
	in := pfx + "kind=blob wall=1 port=nwm::UDS sess=1 cmd=0x000f tag=beacons size=12 hex=4d6f6f6e6b\n" +
		pfx + "kind=blob wall=2 port=nwm::UDS sess=1 cmd=0x001f tag=beacon_netinfo size=264 hex=00aa\n" +
		// ConnectToNetwork's netinfo starts with the host's MAC.
		pfx + "kind=blob wall=3 port=nwm::UDS sess=1 cmd=0x001e tag=netinfo conn_type=1 size=264 hex=98b6e921b8300b01\n" +
		// NodeInfo's friend-code seed is words 2-3 of InitializeWithVersion.
		pfx + `kind=req wall=4 port=nwm::UDS sess=0 cmd=0x001b name="InitializeWithVersion" tid=0 proc="p" hdr=0x001b0302 words=0x001b0302,0x00014000,0x0310beb0,0x0afb0002,0x0061006b` + "\n" +
		pfx + "kind=blob wall=5 port=nwm::UDS sess=1 cmd=0x0017 tag=send dest=0x0002 chan=243 flags=0x02 size=2 hex=0201\n" +
		// The peer's seed, from DecryptBeaconData's node list (node 2 here), and both seeds in
		// the game's own data frames. Only byte-aligned matches count.
		pfx + "kind=blob wall=6 port=nwm::UDS sess=1 cmd=0x001f tag=beacon_nodes size=640 hex=" +
		strings.Repeat("00", 0x28) + "1122334455667788" + strings.Repeat("00", 0x20) + "\n" +
		pfx + "kind=blob wall=7 port=nwm::UDS sess=1 cmd=0x0017 tag=send dest=0x0002 chan=243 flags=0x02 size=12 hex=aab0be1003" + "0200fb0a" + "cc\n" +
		pfx + "kind=blob wall=8 port=nwm::UDS sess=2 cmd=0x0014 tag=recv bind=2 src=0x0002 size=9 hex=011122334455667788\n" +
		pfx + "kind=blob wall=9 port=nwm::UDS sess=2 cmd=0x0014 tag=recv bind=2 src=0x0002 size=9 hex=a1122334455667788b\n" +
		// The relay's address in the shim's notes.
		"[ 0.01] Plugin <Info> shim:Relay:0: relay mode: 203.0.113.7:28300\n" +
		"[ 0.02] Plugin <Info> shim:RelayCheck:0: sockets ready; checking relay.example.net:28300 every 5 s (fake_ndm 0)\n" +
		"[ 0.03] Plugin <Warning> shim:Relay:0: cannot resolve relay.example.net; retrying in 5 s\n" +
		// Stack leftovers above SendTo's 16-bit dest and 8-bit channel (Download Play's held seed
		// pieces); the 32-bit size and the clean flags word stay.
		pfx + `kind=req wall=10 port=nwm::UDS sess=1 cmd=0x0017 name="SendTo" tid=0 proc="p" hdr=0x00170182 words=0x00170182,0x00000001,0x03100002,0x0afb0002,0x00000005,0x00000014,0x00000001,0x00051402,0x001350a8` + "\n"
	var out bytes.Buffer
	n, err := Redact(strings.NewReader(in), &out)
	if err != nil || n != 11 {
		t.Fatalf("Redact = %d, %v; want 11 lines", n, err)
	}
	if !strings.Contains(out.String(), "redacted=padding words=0x00170182,0x00000001,0x00000002,0x00000002,0x00000005,0x00000014,0x00000001,0x00051402,0x001350a8\n") {
		t.Errorf("SendTo padding not masked:\n%s", out.String())
	}
	for _, leak := range []string{"203.0.113.7", "relay.example.net"} {
		if strings.Contains(out.String(), leak) {
			t.Errorf("relay address %s left in", leak)
		}
	}
	for _, want := range []string{"relay mode: <relay>\n", "checking <relay> every 5 s", "cannot resolve <relay>; retrying"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	got := out.String()
	for _, want := range []string{
		"size=12 redacted=seed hex=aa0000000000000000cc\n",
		"size=9 redacted=seed hex=010000000000000000\n",
		"size=9 hex=a1122334455667788b\n", // not byte-aligned: not the seed
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, secret := range []string{"4d6f6f6e6b", "00aa", "98b6e921b830", "0310beb0", "0afb0002"} {
		if strings.Contains(got, secret) {
			t.Errorf("%s left in:\n%s", secret, got)
		}
	}
	for _, want := range []string{
		"tag=beacons size=12 redacted=beacon hex=\n",
		"size=264 redacted=mac hex=0000000000000b01\n",
		"redacted=seed words=0x001b0302,0x00014000,0x00000000,0x00000000,0x0061006b\n",
		"size=2 hex=0201\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	recs, err := Parse(strings.NewReader(got), "console")
	if err != nil || len(recs) != 13 || recs[0].Size != 12 || len(recs[0].Data) != 0 || recs[3].Words[2] != 0 {
		t.Errorf("redacted log does not parse cleanly: %v %+v", err, recs)
	}
}

func TestRedactNames(t *testing.T) {
	const pfx = "[ 1.0] Service <Info> shim:TraceBlob:0: UDSTRACE v=1 "
	// "Serena" in NodeInfo words 4-8 (the seed in words 2-3 is zero already), then "Calem" as
	// the peer in a GetNodeInformation reply, then both inside game data.
	host := pfx + `kind=req wall=1 port=nwm::UDS sess=0 cmd=0x001b name="InitializeWithVersion" tid=0 proc="p" hdr=0x001b0302 words=0x001b0302,0x00014000,0x00000000,0x00000000,0x00650053,0x00650072,0x0061006e,0x00000000,0x00000000,0x00000000,0x00000000,0x00000000,0x00000400` + "\n" +
		pfx + `kind=rsp wall=2 port=nwm::UDS sess=0 cmd=0x000d name="GetNodeInformation" tid=0 proc="p" hdr=0x000d0b00 words=0x000d0b00,0x00000000,0x00000000,0x00000000,0x00610043,0x0065006c,0x0000006d,0x00000000,0x00000000,0x00000000,0x00000000,0x00000000` + "\n" +
		pfx + "kind=blob wall=3 port=nwm::UDS sess=2 cmd=0x0014 tag=recv bind=2 src=0x0002 size=13 hex=ff430061006c0065006d0000ff\n" +
		pfx + "kind=blob wall=4 port=nwm::UDS sess=1 cmd=0x0017 tag=send dest=0x0002 chan=243 flags=0x02 size=13 hex=53006500720065006e006100ee\n"
	// The client's log: its own name is the host's peer, so it keeps the host's stand-in. Its
	// peer's name, "Serena", is only in its data, and goes too.
	client := pfx + `kind=req wall=1 port=nwm::UDS sess=0 cmd=0x001b name="InitializeWithVersion" tid=0 proc="p" hdr=0x001b0302 words=0x001b0302,0x00014000,0x00000000,0x00000000,0x00610043,0x0065006c,0x0000006d,0x00000000,0x00000000,0x00000000,0x00000000,0x00000000,0x00000400` + "\n" +
		pfx + "kind=blob wall=2 port=nwm::UDS sess=2 cmd=0x0014 tag=recv bind=2 src=0x0001 size=12 hex=53006500720065006e006100\n"
	var hostOut, clientOut bytes.Buffer
	n, err := RedactLogs([]io.Reader{strings.NewReader(host), strings.NewReader(client)}, []io.Writer{&hostOut, &clientOut})
	if err != nil || n[0] != 4 || n[1] != 2 {
		t.Fatalf("RedactLogs = %v, %v; want [4 2] lines", n, err)
	}
	got := hostOut.String() + clientOut.String()
	for _, leak := range []string{"0x00650053", "0x00610043", "430061006c0065006d00", "53006500720065006e006100"} {
		if strings.Contains(got, leak) {
			t.Errorf("%s left in:\n%s", leak, got)
		}
	}
	for _, want := range []string{
		"redacted=name words=0x001b0302,0x00014000,0x00000000,0x00000000,0x00310050,0x00000000,",
		"redacted=name words=0x000d0b00,0x00000000,0x00000000,0x00000000,0x00320050,0x00000000,",
		"redacted=name hex=ff5000320000000000000000ff\n",
		"redacted=name hex=500031000000000000000000ee\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "src=0x0001 size=12 redacted=name hex=500031000000000000000000\n") {
		t.Errorf("peer name in the client's data not replaced:\n%s", got)
	}
	recs, err := Parse(strings.NewReader(clientOut.String()), "client")
	if err != nil || len(recs) != 2 || nodeUsername(recs[0].Words) != "P2" {
		t.Errorf("client username not P2: %v %+v", err, recs)
	}
	// Redacting again changes nothing.
	var again bytes.Buffer
	if n, err := Redact(strings.NewReader(hostOut.String()), &again); err != nil || n != 0 || again.String() != hostOut.String() {
		t.Errorf("second pass changed %d line(s), %v:\n%s", n, err, again.String())
	}
}

func TestResultString(t *testing.T) {
	if ResultString(0) != "OK" {
		t.Error("zero should be OK")
	}
	// Bitfield split: description 0-9, module 10-17, summary 21-26, level 27-31.
	code := uint32(1022) | 65<<10 | 9<<21 | 25<<27
	if got := ResultString(code); !strings.Contains(got, "desc=1022 module=65 summary=9 level=25") {
		t.Errorf("ResultString = %s", got)
	}
}

func TestTimelineFoldsPolls(t *testing.T) {
	sessions := []*Session{loadFixture(t, "host"), loadFixture(t, "client")}
	var quiet, verbose bytes.Buffer
	WriteTimeline(&quiet, sessions, false)
	WriteTimeline(&verbose, sessions, true)

	q := quiet.String()
	if !strings.Contains(q, "GetConnectionStatus x2") {
		t.Errorf("unchanged polls not folded:\n%s", q)
	}
	if strings.Contains(q, "PullPacket(bind=1") {
		t.Errorf("data traffic should be folded by default:\n%s", q)
	}
	v := verbose.String()
	if !strings.Contains(v, "[send a000") || !strings.Contains(v, "signal NWM::BindNodeEvent1") {
		t.Errorf("verbose timeline missing payloads or data events:\n%s", v)
	}
	if strings.Count(v, "\n") <= strings.Count(q, "\n") {
		t.Error("verbose timeline should be longer")
	}
}

func TestSummary(t *testing.T) {
	sessions := []*Session{loadFixture(t, "host"), loadFixture(t, "client")}
	var b bytes.Buffer
	WriteSummary(&b, sessions, "fixture trade")
	out := b.String()
	for _, want := range []string{
		"# UDS command trace: fixture trade",
		"| pokemonxy | Serena |",
		"wlan comm ID `0x00123456`",
		"| nwm::UDS | `0x0015` | SetMaxSendDelay | 1 [1] | 0 |",
		"Not in the design table yet: SetMaxSendDelay (0x0015).",
		"| NWM::connection_status_event | 3 | 1 |",
		"| host | send | 1 | 2 | 0x00 | 4 |",
		"ndm:u EnterExclusiveState(state=LocalCommunications) -> OK",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q", want)
		}
	}
	if t.Failed() {
		t.Log(out)
	}
}
