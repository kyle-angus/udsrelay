package udstrace

import (
	"bytes"
	"strings"
	"testing"
)

// console_host.log is written by the shim core's fake-kernel test (make -C shim/core fixtures),
// so this checks that the C formatter and the Go parser agree.
func TestConsoleFixtureFromShimCore(t *testing.T) {
	s := loadFixture(t, "console_host")

	host := findCall(t, s, PortUDS, CmdBeginHostingNetwork)
	ni, err := ParseNetworkInfo(host.Blob("netinfo").Data)
	if err != nil {
		t.Fatal(err)
	}
	if ni.WlanCommID != 0x00123456 || ni.ID8 != 2 || ni.MaxNodes != 2 {
		t.Errorf("network info = %+v", ni)
	}

	send := findCall(t, s, PortUDS, CmdSendTo)
	if b := send.Blob("send"); b == nil || string(b.Data) != "trade-offer" || b.Extra["dest"] != "0x0002" {
		t.Errorf("send blob = %+v", b)
	}
	pull := findCall(t, s, PortUDS, CmdPullPacket)
	if b := pull.Blob("recv"); b == nil || string(b.Data) != "trade-ack!!" || b.Extra["src"] != "0x0002" {
		t.Errorf("recv blob = %+v", b)
	}
	if got := ReplySummary(pull); got != "OK size=11 src=2" {
		t.Errorf("PullPacket reply = %q", got)
	}
	scan := findCall(t, s, PortUDS, CmdRecvBeaconBroadcastData)
	if b := scan.Blob("beacons"); b == nil || b.Size != 32 {
		t.Errorf("beacons blob = %+v", b)
	}
	for _, c := range s.Calls() {
		if c.AwaitingWake || c.Unimplemented {
			t.Errorf("console call %s should be a plain request/reply pair", c.Name)
		}
		if _, ok := c.Result(); !ok {
			t.Errorf("console call %s has no reply", c.Name)
		}
	}
}

func TestCompareSelfMatches(t *testing.T) {
	a, b := loadFixture(t, "host"), loadFixture(t, "host")
	b.Instance = "again"
	var out bytes.Buffer
	if r := WriteCompare(&out, a, b); !r.Matches() {
		t.Errorf("a trace should match itself: %+v\n%s", r, out.String())
	}
	if !strings.HasPrefix(out.String(), "# Trace comparison: again vs host\n\n**Match.**") {
		t.Errorf("unexpected header:\n%s", out.String())
	}
}

func TestCompareFindsDifferences(t *testing.T) {
	azahar, console := loadFixture(t, "host"), loadFixture(t, "console_host")
	var out bytes.Buffer
	r := WriteCompare(&out, azahar, console)
	text := out.String()
	if r.Matches() {
		t.Fatalf("fixtures differ but compare matched:\n%s", text)
	}
	// The Azahar host never scans; the console fixture does.
	if !contains(r.OnlyInCand, "RecvBeaconBroadcastData") {
		t.Errorf("OnlyInCand = %v", r.OnlyInCand)
	}
	if !contains(r.OnlyInRef, "SetMaxSendDelay") || !contains(r.OnlyInRef, "ndm:u EnterExclusiveState") {
		t.Errorf("OnlyInRef = %v", r.OnlyInRef)
	}
	for _, want := range []string{
		"**Differs.**",
		"| SetMaxSendDelay | 1 | 0 | unimplemented | — | **only in host** |",
		"- ndm:u EnterExclusiveState",
		"+ RecvBeaconBroadcastData",
		"  BeginHostingNetwork",
		"| wlan comm ID | `0x00123456` | `0x00123456` |",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("compare output missing %q\n%s", want, text)
		}
	}
}

func TestLCSDiff(t *testing.T) {
	d := lcsDiff([]string{"a", "b", "c", "d"}, []string{"a", "c", "x", "d"})
	var got []string
	for _, l := range d {
		got = append(got, string(l.Op)+l.Sig)
	}
	want := " a,-b, c,+x, d"
	if strings.Join(got, ",") != want {
		t.Errorf("diff = %q, want %q", strings.Join(got, ","), want)
	}
}

func TestShiftWall(t *testing.T) {
	s := loadFixture(t, "client")
	before := s.Items[0].Wall
	recBefore := s.Records[0].Wall
	s.ShiftWall(-150_000)
	if s.Items[0].Wall != before-150_000 || s.Records[0].Wall != recBefore-150_000 {
		t.Errorf("shift not applied: %d -> %d", before, s.Items[0].Wall)
	}
}

func TestPluginNotesKept(t *testing.T) {
	line := "[   0.000250] Plugin <Info> shim:Attach:0: intercepted nwm::UDS: real 0x00000123, game now holds 0x00000456\n"
	recs, err := Parse(strings.NewReader(line), "a")
	if err != nil || len(recs) != 1 || recs[0].Class != "Plugin" || recs[0].Func != "Attach" {
		t.Errorf("records = %+v, err = %v", recs, err)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
