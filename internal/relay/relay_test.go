package relay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestHeaderRoundTrip(t *testing.T) {
	h := Header{Type: MsgData, Session: 0xdeadbeef, Seq: 0xfffe, Ack: 7}
	b := Packet(h, []byte{1, 2, 3})
	if len(b) != HeaderSize+3 {
		t.Fatalf("len %d", len(b))
	}
	got, p, err := ParseHeader(b)
	if err != nil || got != h || !bytes.Equal(p, []byte{1, 2, 3}) {
		t.Fatalf("got %+v %x %v", got, p, err)
	}
	if _, _, err := ParseHeader(b[:5]); err == nil {
		t.Fatal("short header parsed")
	}
	bad := append([]byte(nil), b...)
	bad[2] = 9
	if _, _, err := ParseHeader(bad); err == nil {
		t.Fatal("wrong version parsed")
	}
	// The largest X/Y frame plus all headers fits a 1500-byte MTU.
	if 20+8+HeaderSize+DataHeaderSize+1349 > 1500 {
		t.Fatal("1349-byte frame does not fit the MTU")
	}
}

func TestSeqAfter(t *testing.T) {
	if !seqAfter(1, 0) || seqAfter(0, 1) || !seqAfter(0, 0xffff) || seqAfter(5, 5) {
		t.Fatal("serial-number comparison")
	}
}

func TestNetworkRoundTrip(t *testing.T) {
	var n Network
	binary.BigEndian.PutUint32(n.NetInfo[niCommID:], 0x00055d10)
	n.NetInfo[niID8] = 1
	n.Bitmask = 0b101
	n.Nodes[0][0] = 0xaa
	n.Nodes[2][0] = 0xcc
	b := appendNetwork(nil, &n)
	if len(b) != NetInfoSize+2+2*NodeInfoSize {
		t.Fatalf("encoded %d bytes", len(b))
	}
	r := reader{b: b}
	got := r.network()
	if r.err != nil || got != n {
		t.Fatalf("round trip differs: %v", r.err)
	}
}

type harness struct {
	t   *testing.T
	srv *Server
}

func newHarness(t *testing.T, cfg Config) *harness {
	t.Helper()
	srv, err := Listen("127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return &harness{t: t, srv: srv}
}

type recorder struct {
	mu      sync.Mutex
	events  []NodeEvent
	packets []Data
	resets  int
}

func (r *recorder) onEvent(e NodeEvent) { r.mu.Lock(); r.events = append(r.events, e); r.mu.Unlock() }
func (r *recorder) onPacket(d Data)     { r.mu.Lock(); r.packets = append(r.packets, d); r.mu.Unlock() }
func (r *recorder) onReset()            { r.mu.Lock(); r.resets++; r.mu.Unlock() }

func (r *recorder) waitEvents(t *testing.T, n int) []NodeEvent {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.events) >= n {
			ev := append([]NodeEvent(nil), r.events...)
			r.mu.Unlock()
			return ev
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("got %d events, want %d: %+v", len(r.events), n, r.events)
	return nil
}

func (h *harness) dial(lobby string, rec *recorder, imp *Impairment) *Client {
	h.t.Helper()
	opts := Options{Lobby: lobby, Retry: 50 * time.Millisecond}
	if rec != nil {
		opts.OnEvent, opts.OnPacket, opts.OnReset = rec.onEvent, rec.onPacket, rec.onReset
	}
	if imp != nil {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			h.t.Fatal(err)
		}
		opts.Conn = Impaired(conn, *imp)
	}
	c, err := Dial(h.srv.Addr().String(), opts)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(c.Kill)
	return c
}

func netInfo(commID uint32, id8, maxNodes uint8) []byte {
	b := make([]byte, NetInfoSize)
	binary.BigEndian.PutUint32(b[niCommID:], commID)
	b[niID8] = id8
	b[niMaxNodes] = maxNodes
	return b
}

func node(name byte) (n [NodeInfoSize]byte) {
	n[8] = name
	return n
}

var pass = [32]byte{1, 2, 3}

func TestScanFilters(t *testing.T) {
	h := newHarness(t, Config{})
	a := h.dial("lobby", nil, nil)
	b := h.dial("lobby", nil, nil)
	other := h.dial("elsewhere", nil, nil)
	scanner := h.dial("lobby", nil, nil)

	idA, err := a.Host(netInfo(0x00055d10, 1, 4), pass, node('a'))
	if err != nil {
		t.Fatal(err)
	}
	idB, err := b.Host(netInfo(0x00055d10, 2, 2), pass, node('b'))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Host(netInfo(0x00055d10, 1, 4), pass, node('o')); err != nil {
		t.Fatal(err)
	}
	ids := func(nets []Network, err error) []uint32 {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var out []uint32
		for _, n := range nets {
			out = append(out, n.NetworkID())
		}
		return out
	}
	eq := func(what string, got []uint32, want ...uint32) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %x want %x", what, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: got %x want %x", what, got, want)
			}
		}
	}
	eq("any", ids(scanner.Scan(0xFFFFFFFF, 0xFF)), idA, idB)
	eq("comm ID, any ID8", ids(scanner.Scan(0x00055d10, 0xFF)), idA, idB)
	eq("ID8 1", ids(scanner.Scan(0x00055d10, 1)), idA)
	eq("ID8 2", ids(scanner.Scan(0x00055d10, 2)), idB)
	eq("other game", ids(scanner.Scan(0x12345678, 0xFF)))
	eq("own network hidden", ids(a.Scan(0xFFFFFFFF, 0xFF)), idB)

	// Another lobby cannot join either.
	if _, err := other.Join(idA, pass, ConnClient, node('o')); !errors.Is(err, StatusNotFound) {
		t.Fatalf("cross-lobby join: %v", err)
	}
}

func TestJoinRules(t *testing.T) {
	h := newHarness(t, Config{})
	hostRec := &recorder{}
	host := h.dial("l", hostRec, nil)
	id, err := host.Host(netInfo(0x00055d10, 2, 2), pass, node('h'))
	if err != nil {
		t.Fatal(err)
	}
	c1 := h.dial("l", nil, nil)
	c2 := h.dial("l", nil, nil)
	if _, err := c1.Join(id, [32]byte{9}, ConnClient, node('1')); !errors.Is(err, StatusBadPassphrase) {
		t.Fatalf("bad passphrase: %v", err)
	}
	if _, err := host.Join(id, pass, ConnClient, node('h')); !errors.Is(err, StatusBadRequest) {
		t.Fatalf("host joining itself: %v", err)
	}
	res, err := c1.Join(id, pass, ConnClient, node('1'))
	if err != nil || res.Node != 2 || res.Network.Bitmask != 3 || res.Network.TotalNodes() != 2 {
		t.Fatalf("join: %+v %v", res.Node, err)
	}
	if res.Network.Nodes[0] != node('h') || res.Network.Nodes[1] != node('1') {
		t.Fatal("node list in JOIN_ACK")
	}
	// PSS networks take two nodes.
	if _, err := c2.Join(id, pass, ConnClient, node('2')); !errors.Is(err, StatusFull) {
		t.Fatalf("join full network: %v", err)
	}
	// Spectators do not take a node.
	spec, err := c2.Join(id, pass, ConnSpectator, node('2'))
	if err != nil || spec.Node != 0 {
		t.Fatalf("spectator: %+v %v", spec, err)
	}
	ev := hostRec.waitEvents(t, 1)
	if ev[0].Kind != EventJoined || ev[0].NetworkID != id || ev[0].Node != 2 || ev[0].Info != node('1') {
		t.Fatalf("host event %+v", ev[0])
	}
	if err := c1.SetAppData([]byte{1}); !errors.Is(err, StatusNotHost) {
		t.Fatal("non-host updated app data")
	}
}

func TestRetriedRequestsAreIdempotent(t *testing.T) {
	h := newHarness(t, Config{})
	raw, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	// Speak the protocol by hand so the same HOST_CREATE can be sent twice.
	send := func(typ uint8, seq uint16, p []byte) (Header, []byte) {
		t.Helper()
		if _, err := raw.WriteTo(Packet(Header{Type: typ, Session: 42, Seq: seq}, p), h.srv.Addr()); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, MaxDatagram)
		raw.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := raw.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		h, payload, err := ParseHeader(buf[:n])
		if err != nil {
			t.Fatal(err)
		}
		return h, append([]byte(nil), payload...)
	}
	send(MsgHello, 1, []byte{1, 'l'})
	create := append(append(append([]byte{}, pass[:]...), make([]byte, NodeInfoSize)...), netInfo(1, 1, 4)...)
	h1, p1 := send(MsgHostCreate, 2, create)
	h2, p2 := send(MsgHostCreate, 2, create)
	if h1.Ack != 2 || h2.Ack != 2 || !bytes.Equal(p1, p2) {
		t.Fatalf("retried HOST_CREATE answered differently: %x vs %x", p1, p2)
	}
	if st := h.srv.Stats(); st.Rooms != 1 {
		t.Fatalf("%d rooms after a retried create", st.Rooms)
	}
	h3, p3 := send(MsgHostCreate, 3, create)
	if h3.Ack != 3 || bytes.Equal(p1, p3) {
		t.Fatal("a new HOST_CREATE got the cached answer")
	}
	if st := h.srv.Stats(); st.Rooms != 1 {
		t.Fatalf("re-hosting left %d rooms, want 1", st.Rooms)
	}
	// Unknown sessions are told to start over.
	hr, _ := func() (Header, []byte) {
		if _, err := raw.WriteTo(Packet(Header{Type: MsgKeepalive, Session: 77, Seq: 1}, nil), h.srv.Addr()); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, MaxDatagram)
		raw.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := raw.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		hh, p, _ := ParseHeader(buf[:n])
		return hh, p
	}()
	if hr.Type != MsgReset {
		t.Fatalf("unknown session got %s", MsgName(hr.Type))
	}
}

func TestDataRouting(t *testing.T) {
	h := newHarness(t, Config{})
	recs := []*recorder{{}, {}, {}}
	host := h.dial("l", recs[0], nil)
	c2 := h.dial("l", recs[1], nil)
	c3 := h.dial("l", recs[2], nil)
	id, _ := host.Host(netInfo(1, 1, 4), pass, node('h'))
	if _, err := c2.Join(id, pass, ConnClient, node('2')); err != nil {
		t.Fatal(err)
	}
	if _, err := c3.Join(id, pass, ConnClient, node('3')); err != nil {
		t.Fatal(err)
	}
	recs[0].waitEvents(t, 2)
	payload := bytes.Repeat([]byte{0x5a}, 1349)
	c2.Send(3, 243, 2, payload) // client to client, via the relay as host would route it
	host.Send(BroadcastNode, 243, 2, []byte("bcast"))
	c3.Send(1, 243, 2, []byte("to host"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recs[0].mu.Lock()
		n0 := len(recs[0].packets)
		recs[0].mu.Unlock()
		recs[1].mu.Lock()
		n1 := len(recs[1].packets)
		recs[1].mu.Unlock()
		recs[2].mu.Lock()
		n2 := len(recs[2].packets)
		recs[2].mu.Unlock()
		if n0 == 1 && n1 == 1 && n2 == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	check := func(r *recorder, i int, src uint16, want []byte) {
		t.Helper()
		r.mu.Lock()
		defer r.mu.Unlock()
		if i >= len(r.packets) {
			t.Fatalf("missing packet %d", i)
		}
		p := r.packets[i]
		if p.Src != src || p.Channel != 243 || p.Flags != 2 || !bytes.Equal(p.Payload, want) {
			t.Fatalf("packet from %d: %+v", src, p)
		}
	}
	// c3 gets c2's frame and the broadcast, in some order.
	recs[2].mu.Lock()
	if len(recs[2].packets) == 2 && recs[2].packets[0].Src == 1 {
		recs[2].packets[0], recs[2].packets[1] = recs[2].packets[1], recs[2].packets[0]
	}
	recs[2].mu.Unlock()
	check(recs[2], 0, 2, payload)
	check(recs[2], 1, 1, []byte("bcast"))
	check(recs[1], 0, 1, []byte("bcast"))
	check(recs[0], 0, 3, []byte("to host"))
}

func TestEjectAndClose(t *testing.T) {
	h := newHarness(t, Config{})
	hostRec, r2, r3 := &recorder{}, &recorder{}, &recorder{}
	host := h.dial("l", hostRec, nil)
	c2 := h.dial("l", r2, nil)
	c3 := h.dial("l", r3, nil)
	id, _ := host.Host(netInfo(1, 1, 4), pass, node('h'))
	c2.Join(id, pass, ConnClient, node('2'))
	c3.Join(id, pass, ConnClient, node('3'))
	if err := c2.Eject(3); !errors.Is(err, StatusNotHost) {
		t.Fatalf("client eject: %v", err)
	}
	if err := host.Eject(3); err != nil {
		t.Fatal(err)
	}
	ev3 := r3.waitEvents(t, 1)
	if ev3[0].Kind != EventEjected {
		t.Fatalf("ejected node got %+v", ev3[0])
	}
	ev2 := r2.waitEvents(t, 2) // node 3 joined, node 3 left
	if ev2[1].Kind != EventLeft || ev2[1].Node != 3 || ev2[1].Reason != ReasonEjected || ev2[1].Bitmask != 3 {
		t.Fatalf("peer of ejected node got %+v", ev2[1])
	}
	if err := host.Leave(); err != nil {
		t.Fatal(err)
	}
	ev2 = r2.waitEvents(t, 3)
	if ev2[2].Kind != EventClosed {
		t.Fatalf("client of departed host got %+v", ev2[2])
	}
	if nets, _ := c3.Scan(0xFFFFFFFF, 0xFF); len(nets) != 0 {
		t.Fatal("closed network still listed")
	}
}

// Node events must arrive exactly once and in order on a lossy link.
func TestEventsSurviveLoss(t *testing.T) {
	h := newHarness(t, Config{Retransmit: 20 * time.Millisecond})
	hostRec := &recorder{}
	host := h.dial("l", hostRec, &Impairment{Loss: 0.3, Seed: 7})
	id, err := host.Host(netInfo(1, 1, 16), pass, node('h'))
	if err != nil {
		t.Fatal(err)
	}
	const joiners = 10
	var cs []*Client
	for i := 0; i < joiners; i++ {
		c := h.dial("l", nil, nil)
		if _, err := c.Join(id, pass, ConnClient, node(byte('a'+i))); err != nil {
			t.Fatal(err)
		}
		cs = append(cs, c)
	}
	for _, c := range cs {
		if err := c.Leave(); err != nil {
			t.Fatal(err)
		}
	}
	ev := hostRec.waitEvents(t, 2*joiners)
	time.Sleep(100 * time.Millisecond)
	hostRec.mu.Lock()
	defer hostRec.mu.Unlock()
	if len(hostRec.events) != 2*joiners {
		t.Fatalf("got %d events, want %d (duplicates?)", len(hostRec.events), 2*joiners)
	}
	for i := 0; i < joiners; i++ {
		if ev[i].Kind != EventJoined || ev[i].Node != uint16(i+2) {
			t.Fatalf("event %d: %+v", i, ev[i])
		}
		if e := ev[joiners+i]; e.Kind != EventLeft || e.Node != uint16(i+2) {
			t.Fatalf("event %d: %+v", joiners+i, e)
		}
	}
	if ev[2*joiners-1].Bitmask != 1 {
		t.Fatalf("final bitmask %#x", ev[2*joiners-1].Bitmask)
	}
}

func TestTimeoutDropsSilentNode(t *testing.T) {
	h := newHarness(t, Config{Timeout: 300 * time.Millisecond, KeepaliveInterval: 50 * time.Millisecond})
	hostRec := &recorder{}
	host := h.dial("l", hostRec, nil)
	id, _ := host.Host(netInfo(1, 1, 4), pass, node('h'))
	c := h.dial("l", nil, nil)
	c.Join(id, pass, ConnClient, node('c'))
	// Keepalives alone keep an idle node in the room.
	time.Sleep(600 * time.Millisecond)
	if len(hostRec.waitEvents(t, 1)) != 1 {
		t.Fatal("idle node dropped despite keepalives")
	}
	start := time.Now()
	c.Kill()
	ev := hostRec.waitEvents(t, 2)
	if ev[1].Kind != EventLeft || ev[1].Reason != ReasonTimeout || ev[1].Bitmask != 1 {
		t.Fatalf("timeout event %+v", ev[1])
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("timeout took %v", d)
	}
	if host.RTT() == 0 {
		t.Fatal("no keepalive RTT measured")
	}
}

func TestResetAfterServerRestart(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	srv := NewServer(conn, Config{})
	go srv.Serve()
	rec := &recorder{}
	c, err := Dial(addr, Options{Lobby: "l", Keepalive: 30 * time.Millisecond, OnReset: rec.onReset})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Kill()
	srv.Close()
	conn2, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Skipf("cannot rebind %s: %v", addr, err)
	}
	srv2 := NewServer(conn2, Config{})
	go srv2.Serve()
	defer srv2.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		n := rec.resets
		rec.mu.Unlock()
		if n > 0 && srv2.Stats().Sessions == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rec.resets == 0 || srv2.Stats().Sessions != 1 {
		t.Fatalf("resets %d, sessions %d", rec.resets, srv2.Stats().Sessions)
	}
	if _, err := c.Host(netInfo(1, 1, 4), pass, node('h')); err != nil {
		t.Fatalf("host after reset: %v", err)
	}
}
