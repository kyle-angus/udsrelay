package udsfake

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"time"

	"udsrelay/internal/relay"
)

// Values from the Pokémon X/Y traces.
const (
	xyCommID  = 0x00055d10
	tradeID8  = 0x01
	pssID8    = 0x02
	xyChannel = 243
	xyFlags   = 0x02
)

var xyPassphrase = []byte("GNgBwMcVotYs\x00")

// startRelay runs a relay serving the given lobby keys.
func startRelay(t *testing.T, cfg relay.Config, lobbies ...string) *relay.Server {
	t.Helper()
	for _, k := range lobbies {
		cfg.Lobbies = append(cfg.Lobbies, relay.Lobby{Name: k, Key: k})
	}
	srv, err := relay.Listen("127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return srv
}

func newConsole(t *testing.T, srv *relay.Server, lobby, user string, seed uint64) *Console {
	t.Helper()
	c, err := New(Config{Relay: srv.Addr().String(), Lobby: lobby})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Kill() })
	if r := c.InitializeWithVersion(MakeNodeInfo(user, seed)); r != ResultOK {
		t.Fatalf("init: %#x", r)
	}
	return c
}

func mustOK(t *testing.T, what string, r uint32) {
	t.Helper()
	if r != ResultOK {
		t.Fatalf("%s: result %#08x", what, r)
	}
}

func wantStatus(t *testing.T, c *Console, what string, want ConnectionStatus) {
	t.Helper()
	ok := c.WaitStatus(func(st ConnectionStatus) bool {
		return st.Status == want.Status && st.NodeBitmask == want.NodeBitmask && st.TotalNodes == want.TotalNodes
	}, 3*time.Second)
	got := c.GetConnectionStatus()
	if !ok || got != want {
		t.Fatalf("%s: status %+v, want %+v", what, got, want)
	}
}

// TestPhase3Exit is the Phase 3 exit criterion: two fake consoles host, scan, join, exchange
// data and see leave events through the relay.
func TestPhase3Exit(t *testing.T) {
	srv := startRelay(t, relay.Config{Timeout: 600 * time.Millisecond, KeepaliveInterval: 100 * time.Millisecond}, "exit")
	host := newConsole(t, srv, "exit", "Kangus", 0x1111)
	guest := newConsole(t, srv, "exit", "kangus", 0x2222)

	// Commands X/Y calls that Azahar leaves unimplemented answer as hardware does.
	mustOK(t, "SetProbeResponseParam", host.SetProbeResponseParam())
	if r, w := host.Cmd0023(); r != ResultOK || w != 0x00881fff {
		t.Fatalf("0x0023: %#x %#x", r, w)
	}
	if r := host.DestroyNetwork(); r != ResultInvalidState {
		t.Fatalf("DestroyNetwork with no network: %#08x, want 0xC8A113EA", r)
	}

	// Host: a trade network, ID8 0x01, up to 4 nodes, app data set afterwards as the game does.
	mustOK(t, "Bind host", host.Bind(2, xyChannel, relay.BroadcastNode, 0x2e30))
	mustOK(t, "BeginHostingNetwork", host.BeginHostingNetwork(MakeNetInfo(xyCommID, tradeID8, 4, 0x8000, nil), xyPassphrase))
	wantStatus(t, host, "host after hosting", ConnectionStatus{Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished,
		NodeID: 1, ChangedNodes: 1, TotalNodes: 1, MaxNodes: 4, NodeBitmask: 1})
	if r, ch := host.GetChannel(); r != ResultOK || ch != 11 {
		t.Fatalf("GetChannel: %#x %d", r, ch)
	}
	appData := make([]byte, 184)
	for i := range appData {
		appData[i] = byte(i*7 + 1)
	}
	mustOK(t, "SetApplicationData", host.SetApplicationData(appData))

	// Guest: scan for ID8 0x01 and find the host's network with its app data byte for byte.
	nets, r := guest.RecvBeaconBroadcastData(xyCommID, tradeID8)
	mustOK(t, "scan", r)
	if len(nets) != 1 {
		t.Fatalf("scan found %d networks, want 1", len(nets))
	}
	n := nets[0]
	if n.NetworkID() == 0 || n.NetworkID() != host.HostedNetworkIDs()[0] {
		t.Fatalf("network ID %#x, host has %#x", n.NetworkID(), host.HostedNetworkIDs())
	}
	if n.CommID() != xyCommID || n.ID8() != tradeID8 || n.MaxNodes() != 4 || n.TotalNodes() != 1 {
		t.Fatalf("scanned network: comm %#x id8 %#x max %d total %d", n.CommID(), n.ID8(), n.MaxNodes(), n.TotalNodes())
	}
	if !bytes.Equal(n.AppData(), appData) {
		t.Fatalf("app data changed in transit")
	}
	nodes, r := guest.DecryptBeaconData(n.NetworkID())
	mustOK(t, "DecryptBeaconData", r)
	if len(nodes) != 1 || NodeUsername(nodes[0]) != "Kangus" {
		t.Fatalf("beacon node list: %d nodes", len(nodes))
	}
	// A PSS-style scan for ID8 0x02 must not see the trade network.
	if nets, _ := guest.RecvBeaconBroadcastData(xyCommID, pssID8); len(nets) != 0 {
		t.Fatalf("ID8 0x02 scan found %d networks", len(nets))
	}

	// Join with the wrong passphrase fails; the right one makes the guest node 2.
	if r := guest.ConnectToNetwork(n.NetInfo[:], []byte("wrong"), relay.ConnClient); r == ResultOK {
		t.Fatal("join with wrong passphrase succeeded")
	}
	mustOK(t, "Bind guest", guest.Bind(2, xyChannel, relay.BroadcastNode, 0x2e30))
	mustOK(t, "ConnectToNetwork", guest.ConnectToNetwork(n.NetInfo[:], xyPassphrase, relay.ConnClient))
	wantStatus(t, guest, "guest after join", ConnectionStatus{Status: StatusConnectedAsClient, Reason: ReasonConnectionEstablished,
		NodeID: 2, ChangedNodes: 3, TotalNodes: 2, MaxNodes: 4, NodeBitmask: 3})
	if st := guest.GetConnectionStatus(); st.ChangedNodes != 0 {
		t.Fatalf("changed nodes not cleared by read: %#x", st.ChangedNodes)
	}
	wantStatus(t, host, "host after join", ConnectionStatus{Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished,
		NodeID: 1, ChangedNodes: 2, TotalNodes: 2, MaxNodes: 4, NodeBitmask: 3})
	if info, ok := host.NodeInfo(2); !ok || NodeUsername(info) != "kangus" {
		t.Fatal("host has no NodeInfo for node 2")
	}
	if r := guest.DestroyNetwork(); r != ResultInvalidState {
		t.Fatalf("client DestroyNetwork: %#08x, want 0xC8A113EA", r)
	}

	// Data both ways at X/Y sizes, node to node with flags 0x02, then a broadcast.
	rng := rand.New(rand.NewPCG(1, 2))
	sizes := []int{46, 48, 65, 69, 80, 512, 1349}
	exchange := func(from, to *Console, fromNode, dst uint16) {
		t.Helper()
		for _, size := range sizes {
			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(rng.Uint32())
			}
			mustOK(t, "SendTo", from.SendTo(dst, xyChannel, xyFlags, payload))
			p, ok, r := to.WaitPacket(2, 0x5dc, 2*time.Second)
			if !ok || r != ResultOK {
				t.Fatalf("%d-byte packet from node %d did not arrive (result %#x)", size, fromNode, r)
			}
			if p.Src != fromNode || p.Channel != xyChannel || !bytes.Equal(p.Data, payload) {
				t.Fatalf("%d-byte packet from node %d arrived as src %d chan %d, %d bytes, equal=%v",
					size, fromNode, p.Src, p.Channel, len(p.Data), bytes.Equal(p.Data, payload))
			}
		}
	}
	exchange(host, guest, 1, 2)
	exchange(guest, host, 2, 1)
	exchange(host, guest, 1, relay.BroadcastNode)
	if _, ok, r := host.PullPacket(2, 0x5dc); ok || r != ResultOK {
		t.Fatalf("host queue not empty after exchange")
	}

	// Clean leave: the guest disconnects and the host sees node 2 go.
	signals := host.StatusSignals()
	mustOK(t, "DisconnectNetwork", guest.DisconnectNetwork())
	wantStatus(t, guest, "guest after disconnect", ConnectionStatus{Status: StatusNotConnected, Reason: ReasonLeft, NodeID: 2})
	if _, _, r := guest.PullPacket(2, 0x5dc); r != ResultInvalidState {
		t.Fatalf("PullPacket after disconnect: %#08x, want 0xC8A113EA", r)
	}
	wantStatus(t, host, "host after guest left", ConnectionStatus{Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished,
		NodeID: 1, ChangedNodes: 2, TotalNodes: 1, MaxNodes: 4, NodeBitmask: 1})
	if host.StatusSignals() <= signals {
		t.Fatal("host status event not signalled for the leave")
	}

	// A third console joins and vanishes without a word; the relay times it out and the host
	// sees the leave long before the game's own 23–28 s timeout.
	third := newConsole(t, srv, "exit", "third", 0x3333)
	nets, _ = third.RecvBeaconBroadcastData(xyCommID, tradeID8)
	if len(nets) != 1 {
		t.Fatalf("third console found %d networks", len(nets))
	}
	mustOK(t, "third ConnectToNetwork", third.ConnectToNetwork(nets[0].NetInfo[:], xyPassphrase, relay.ConnClient))
	if st := third.GetConnectionStatus(); st.NodeID != 2 {
		t.Fatalf("third console got node %d, want the freed node 2", st.NodeID)
	}
	wantStatus(t, host, "host after third joined", ConnectionStatus{Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished,
		NodeID: 1, ChangedNodes: 2, TotalNodes: 2, MaxNodes: 4, NodeBitmask: 3})
	vanished := time.Now()
	third.Kill()
	wantStatus(t, host, "host after third vanished", ConnectionStatus{Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished,
		NodeID: 1, ChangedNodes: 2, TotalNodes: 1, MaxNodes: 4, NodeBitmask: 1})
	t.Logf("vanished node dropped after %v", time.Since(vanished).Round(time.Millisecond))

	// The guest rejoins; the host destroys the network and the guest loses it.
	nets, _ = guest.RecvBeaconBroadcastData(xyCommID, tradeID8)
	if len(nets) != 1 {
		t.Fatalf("rejoin scan found %d networks", len(nets))
	}
	mustOK(t, "rejoin", guest.ConnectToNetwork(nets[0].NetInfo[:], xyPassphrase, relay.ConnClient))
	wantStatus(t, host, "host after rejoin", ConnectionStatus{Status: StatusConnectedAsHost, Reason: ReasonConnectionEstablished,
		NodeID: 1, ChangedNodes: 2, TotalNodes: 2, MaxNodes: 4, NodeBitmask: 3})
	mustOK(t, "DestroyNetwork", host.DestroyNetwork())
	wantStatus(t, host, "host after destroy", ConnectionStatus{Status: StatusNotConnected, Reason: ReasonLeft, NodeID: 1})
	wantStatus(t, guest, "guest after host left", ConnectionStatus{Status: StatusNotConnected, Reason: ReasonConnectionLost, NodeID: 2})
	if _, _, r := host.PullPacket(2, 0x5dc); r != ResultInvalidState {
		t.Fatalf("host PullPacket after destroy: %#08x", r)
	}
	if r := host.DestroyNetwork(); r != ResultInvalidState {
		t.Fatalf("second DestroyNetwork: %#08x, want 0xC8A113EA", r)
	}
	if nets, _ := guest.RecvBeaconBroadcastData(xyCommID, 0xFF); len(nets) != 0 {
		t.Fatalf("destroyed network still listed")
	}

	// Each hosting gets its own random non-zero network ID.
	mustOK(t, "host again", host.BeginHostingNetwork(MakeNetInfo(xyCommID, pssID8, 2, 0x8000, appData), xyPassphrase))
	ids := host.HostedNetworkIDs()
	if len(ids) != 2 || ids[0] == 0 || ids[1] == 0 || ids[0] == ids[1] {
		t.Fatalf("network IDs %#x", ids)
	}
	st := srv.Stats()
	if st.DataDropped != 0 {
		t.Errorf("relay dropped %d data packets", st.DataDropped)
	}
	t.Logf("relay stats: %+v", st)
}
