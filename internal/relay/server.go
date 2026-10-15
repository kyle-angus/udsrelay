package relay

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"sort"
	"sync"
	"time"
)

// Config tunes the server. Zero fields take the defaults.
type Config struct {
	// KeepaliveInterval is advertised to consoles in HELLO_ACK. Default 1 s.
	KeepaliveInterval time.Duration
	// Timeout drops a session that has sent nothing for this long; its peers get a node-left
	// event. Default 5 s: well inside Pokémon X/Y's own 23–28 s peer timeout, and twice the
	// longest gap between the game's 2 s heartbeats, though keepalives arrive every second anyway.
	Timeout time.Duration
	// Retransmit is how often an unacknowledged NODE_EVENT is resent. Default 200 ms.
	Retransmit time.Duration
	Logger     *slog.Logger
}

func (c *Config) setDefaults() {
	if c.KeepaliveInterval == 0 {
		c.KeepaliveInterval = time.Second
	}
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	if c.Retransmit == 0 {
		c.Retransmit = 200 * time.Millisecond
	}
	if c.Logger == nil {
		c.Logger = slog.New(discard{})
	}
}

// Server is the relay. All state sits behind one mutex; traffic is a few packets per second
// per console, so contention does not matter.
type Server struct {
	conn net.PacketConn
	cfg  Config
	log  *slog.Logger

	mu       sync.Mutex
	sessions map[uint32]*session
	rooms    map[uint32]*room // by network ID
	stats    Stats
	done     chan struct{}
	closed   bool
}

// Stats counts server activity since start.
type Stats struct {
	Sessions, Rooms        int // current
	DataIn, DataOut        uint64
	DataDropped            uint64 // undeliverable: no such node, sender not in a room, spectator sender
	Timeouts               uint64
	EventsSent, EventsAckd uint64
}

type session struct {
	id       uint32
	addr     net.Addr
	lobby    string
	lastSeen time.Time

	room      *room
	node      uint16 // 0 while not in a room or as a spectator
	spectator bool

	// Request dedup: a retried request gets the cached response.
	lastReqSeq uint16
	haveReq    bool
	lastResp   [][]byte

	// Reliable NODE_EVENT stream, stop-and-wait.
	evSeq    uint16
	evQueue  [][]byte // payloads waiting their turn
	inflight []byte   // full datagram awaiting ACK
	inflSeq  uint16
	inflSent time.Time
	outSeq   uint16 // seq for unreliable datagrams to this session
}

type room struct {
	net      Network
	lobby    string
	passHash [32]byte
	host     *session
	members  [MaxNodes]*session // index node-1
	specs    map[*session]bool
	created  time.Time
	packets  uint64
	bytes    uint64
	joins    int
}

func (r *room) id() uint32 { return r.net.NetworkID() }

func (r *room) total() uint8 {
	n := uint8(0)
	for _, m := range r.members {
		if m != nil {
			n++
		}
	}
	return n
}

func (r *room) setNodes() {
	r.net.NetInfo[niTotalNodes] = r.total()
}

// NewServer wraps a bound packet conn. Call Serve to run it.
func NewServer(conn net.PacketConn, cfg Config) *Server {
	cfg.setDefaults()
	return &Server{
		conn: conn, cfg: cfg, log: cfg.Logger,
		sessions: map[uint32]*session{}, rooms: map[uint32]*room{}, done: make(chan struct{}),
	}
}

// Listen binds addr (e.g. ":28300") and returns a server ready to Serve.
func Listen(addr string, cfg Config) (*Server, error) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}
	return NewServer(conn, cfg), nil
}

// Addr is the bound address.
func (s *Server) Addr() net.Addr { return s.conn.LocalAddr() }

// Serve handles datagrams until Close. It returns nil after Close.
func (s *Server) Serve() error {
	go s.tick()
	buf := make([]byte, MaxDatagram)
	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed || errors.Is(err, net.ErrClosed) {
				return nil
			}
			s.log.Warn("read", "err", err)
			continue
		}
		s.handle(buf[:n], addr)
	}
}

// Close stops the server.
func (s *Server) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.done)
	}
	s.mu.Unlock()
	return s.conn.Close()
}

// Stats returns a snapshot of the counters.
func (s *Server) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stats
	st.Sessions, st.Rooms = len(s.sessions), len(s.rooms)
	return st
}

func (s *Server) tick() {
	t := time.NewTicker(s.cfg.Retransmit / 2)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case now := <-t.C:
			s.mu.Lock()
			for _, ss := range s.sessions {
				if now.Sub(ss.lastSeen) > s.cfg.Timeout {
					s.stats.Timeouts++
					s.log.Info("session timed out", "session", hex32(ss.id), "addr", ss.addr, "silent", now.Sub(ss.lastSeen).Round(time.Millisecond))
					s.leaveRoom(ss, ReasonTimeout)
					delete(s.sessions, ss.id)
					continue
				}
				if ss.inflight != nil && now.Sub(ss.inflSent) >= s.cfg.Retransmit {
					s.write(ss, ss.inflight)
					ss.inflSent = now
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Server) write(ss *session, b []byte) {
	if _, err := s.conn.WriteTo(b, ss.addr); err != nil {
		s.log.Debug("write", "session", hex32(ss.id), "err", err)
	}
}

func (s *Server) reply(ss *session, typ uint8, ack uint16, payload []byte) []byte {
	ss.outSeq++
	b := Packet(Header{Type: typ, Session: ss.id, Seq: ss.outSeq, Ack: ack}, payload)
	s.write(ss, b)
	return b
}

func isRequest(t uint8) bool {
	switch t {
	case MsgHello, MsgHostCreate, MsgHostUpdate, MsgScanReq, MsgJoinReq, MsgEject, MsgLeave:
		return true
	}
	return false
}

func (s *Server) handle(b []byte, addr net.Addr) {
	h, p, err := ParseHeader(b)
	if err != nil || h.Session == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	ss := s.sessions[h.Session]
	if ss == nil {
		if h.Type != MsgHello {
			// Unknown session: the server restarted or timed it out. Tell it to start over.
			s.write(&session{id: h.Session, addr: addr}, Packet(Header{Type: MsgReset, Session: h.Session, Ack: h.Seq}, nil))
			return
		}
		r := reader{b: p}
		key := string(r.take(int(r.u8())))
		if r.err != nil || len(key) > MaxLobbyKey {
			return
		}
		ss = &session{id: h.Session, addr: addr, lobby: key}
		s.sessions[h.Session] = ss
		s.log.Info("session", "session", hex32(ss.id), "addr", addr, "lobby", key)
	}
	ss.lastSeen = time.Now()
	if ss.addr.String() != addr.String() {
		s.log.Info("session moved", "session", hex32(ss.id), "from", ss.addr, "to", addr)
		ss.addr = addr
	}
	if isRequest(h.Type) {
		if ss.haveReq && h.Seq == ss.lastReqSeq {
			for _, r := range ss.lastResp {
				s.write(ss, r)
			}
			return
		}
		resp := s.request(ss, h, p)
		ss.haveReq, ss.lastReqSeq, ss.lastResp = true, h.Seq, resp
		return
	}
	switch h.Type {
	case MsgData:
		s.data(ss, p)
	case MsgKeepalive:
		s.reply(ss, MsgKeepaliveAck, h.Seq, p)
	case MsgAck:
		if ss.inflight != nil && h.Ack == ss.inflSeq {
			ss.inflight = nil
			s.stats.EventsAckd++
			s.sendNextEvent(ss)
		}
	}
}

// request handles one new request and returns the datagrams it sent, for the dedup cache.
func (s *Server) request(ss *session, h Header, p []byte) [][]byte {
	one := func(typ uint8, payload []byte) [][]byte {
		return [][]byte{s.reply(ss, typ, h.Seq, payload)}
	}
	status := func(typ uint8, st Status) [][]byte { return one(typ, []byte{byte(st)}) }
	r := reader{b: p}
	switch h.Type {
	case MsgHello:
		out := []byte{byte(StatusOK)}
		out = binary.BigEndian.AppendUint16(out, uint16(s.cfg.KeepaliveInterval/time.Millisecond))
		out = binary.BigEndian.AppendUint16(out, uint16(s.cfg.Timeout/time.Millisecond))
		return one(MsgHelloAck, out)

	case MsgHostCreate:
		var hash [32]byte
		copy(hash[:], r.take(32))
		info := r.nodeInfo()
		ni := r.take(NetInfoSize)
		if r.err != nil {
			return status(MsgHostAck, StatusBadRequest)
		}
		s.leaveRoom(ss, ReasonClean)
		rm := &room{lobby: ss.lobby, passHash: hash, host: ss, specs: map[*session]bool{}, created: time.Now()}
		copy(rm.net.NetInfo[:], ni)
		if m := rm.net.NetInfo[niMaxNodes]; m == 0 || m > MaxNodes {
			rm.net.NetInfo[niMaxNodes] = MaxNodes
		}
		var id uint32
		for id == 0 || s.rooms[id] != nil {
			id = rand.Uint32()
		}
		binary.BigEndian.PutUint32(rm.net.NetInfo[niNetworkID:], id)
		rm.members[0] = ss
		rm.net.Bitmask = 1
		rm.net.Nodes[0] = info
		rm.setNodes()
		s.rooms[id] = rm
		ss.room, ss.node, ss.spectator = rm, 1, false
		// Debug: PSS hosts and destroys an empty network every 1–3 s in the background.
		s.log.Debug("network up", "network", hex32(id), "host", hex32(ss.id), "comm_id", hex32(rm.net.CommID()),
			"id8", rm.net.ID8(), "max_nodes", rm.net.MaxNodes(), "lobby", ss.lobby)
		return one(MsgHostAck, binary.BigEndian.AppendUint32([]byte{byte(StatusOK)}, id))

	case MsgHostUpdate:
		rm := ss.room
		if rm == nil || rm.host != ss {
			return status(MsgUpdateAck, StatusNotHost)
		}
		switch r.u8() {
		case UpdateAppData:
			data := r.take(int(r.u8()))
			if r.err != nil || len(data) > MaxAppData {
				return status(MsgUpdateAck, StatusBadRequest)
			}
			clear(rm.net.NetInfo[niAppData : niAppData+MaxAppData])
			copy(rm.net.NetInfo[niAppData:], data)
			rm.net.NetInfo[niAppDataSize] = byte(len(data))
		case UpdateAttributes:
			a := r.u16()
			if r.err != nil {
				return status(MsgUpdateAck, StatusBadRequest)
			}
			binary.BigEndian.PutUint16(rm.net.NetInfo[niAttributes:], a)
		default:
			return status(MsgUpdateAck, StatusBadRequest)
		}
		return status(MsgUpdateAck, StatusOK)

	case MsgScanReq:
		commID, id8 := r.u32(), r.u8()
		if r.err != nil {
			return status(MsgScanResp, StatusBadRequest)
		}
		var found []*room
		for _, rm := range s.rooms {
			if rm.lobby != ss.lobby || rm.host == ss {
				continue
			}
			if commID != 0xFFFFFFFF && rm.net.CommID() != commID {
				continue
			}
			if id8 != 0xFF && rm.net.ID8() != id8 {
				continue
			}
			found = append(found, rm)
		}
		sort.Slice(found, func(i, j int) bool { return found[i].created.Before(found[j].created) })
		if len(found) > 255 {
			found = found[:255]
		}
		if len(found) == 0 {
			return one(MsgScanResp, []byte{byte(StatusOK), 0, 0})
		}
		// One datagram per network, like one beacon per network on the air: a lost one just
		// means that network is missing from this scan.
		var out [][]byte
		for i, rm := range found {
			out = append(out, s.reply(ss, MsgScanResp, h.Seq, appendNetwork([]byte{byte(StatusOK), byte(i), byte(len(found))}, &rm.net)))
		}
		return out

	case MsgJoinReq:
		id := r.u32()
		var hash [32]byte
		copy(hash[:], r.take(32))
		connType := r.u8()
		info := r.nodeInfo()
		if r.err != nil {
			return status(MsgJoinAck, StatusBadRequest)
		}
		rm := s.rooms[id]
		if rm == nil || rm.lobby != ss.lobby {
			return status(MsgJoinAck, StatusNotFound)
		}
		if rm.host == ss {
			return status(MsgJoinAck, StatusBadRequest)
		}
		if rm.passHash != hash {
			return status(MsgJoinAck, StatusBadPassphrase)
		}
		if ss.room != rm {
			s.leaveRoom(ss, ReasonClean)
		}
		ack := func(node uint16) [][]byte {
			out := binary.BigEndian.AppendUint16([]byte{byte(StatusOK)}, node)
			return one(MsgJoinAck, appendNetwork(out, &rm.net))
		}
		if connType == ConnSpectator {
			rm.specs[ss] = true
			ss.room, ss.node, ss.spectator = rm, 0, true
			s.log.Info("spectator joined", "network", hex32(id), "session", hex32(ss.id))
			return ack(0)
		}
		if ss.room == rm && ss.node != 0 {
			return ack(ss.node)
		}
		node := uint16(0)
		for i := 1; i < int(rm.net.MaxNodes()); i++ {
			if rm.members[i] == nil {
				node = uint16(i + 1)
				break
			}
		}
		if node == 0 {
			return status(MsgJoinAck, StatusFull)
		}
		rm.members[node-1] = ss
		rm.joins++
		rm.net.Bitmask |= 1 << (node - 1)
		rm.net.Nodes[node-1] = info
		rm.setNodes()
		ss.room, ss.node, ss.spectator = rm, node, false
		s.log.Info("node joined", "network", hex32(id), "node", node, "session", hex32(ss.id), "nodes", rm.total())
		out := ack(node)
		s.broadcastEvent(rm, ss, NodeEvent{Kind: EventJoined, NetworkID: id, Node: node, Bitmask: rm.net.Bitmask, TotalNodes: rm.total(), Info: info})
		return out

	case MsgEject:
		node := r.u16()
		rm := ss.room
		if r.err != nil {
			return status(MsgEjectAck, StatusBadRequest)
		}
		if rm == nil || rm.host != ss {
			return status(MsgEjectAck, StatusNotHost)
		}
		if node == 0 { // EjectSpectators
			for sp := range rm.specs {
				s.queueEvent(sp, NodeEvent{Kind: EventEjected, NetworkID: rm.id(), Bitmask: rm.net.Bitmask, TotalNodes: rm.total()})
				sp.room, sp.spectator = nil, false
			}
			clear(rm.specs)
			return status(MsgEjectAck, StatusOK)
		}
		if node < 2 || node > MaxNodes || rm.members[node-1] == nil {
			return status(MsgEjectAck, StatusNotFound)
		}
		target := rm.members[node-1]
		s.queueEvent(target, NodeEvent{Kind: EventEjected, NetworkID: rm.id(), Node: node})
		s.leaveRoom(target, ReasonEjected)
		return status(MsgEjectAck, StatusOK)

	case MsgLeave:
		s.leaveRoom(ss, ReasonClean)
		return status(MsgLeaveAck, StatusOK)
	}
	return nil
}

// leaveRoom takes ss out of its room. A departing host closes the room.
func (s *Server) leaveRoom(ss *session, reason uint8) {
	rm := ss.room
	if rm == nil {
		return
	}
	ss.room, ss.node, ss.spectator = nil, 0, false
	if rm.specs[ss] {
		delete(rm.specs, ss)
		return
	}
	if rm.host == ss {
		for i, m := range rm.members {
			if m != nil && m != ss {
				s.queueEvent(m, NodeEvent{Kind: EventClosed, Reason: reason, NetworkID: rm.id(), Node: 1})
				m.room, m.node = nil, 0
			}
			rm.members[i] = nil
		}
		for sp := range rm.specs {
			s.queueEvent(sp, NodeEvent{Kind: EventClosed, Reason: reason, NetworkID: rm.id(), Node: 1})
			sp.room, sp.spectator = nil, false
		}
		delete(s.rooms, rm.id())
		level := slog.LevelDebug
		if rm.packets > 0 || rm.joins > 0 {
			level = slog.LevelInfo
		}
		s.log.Log(context.Background(), level, "network down", "network", hex32(rm.id()), "reason", reasonName(reason),
			"age", time.Since(rm.created).Round(time.Millisecond), "packets", rm.packets, "bytes", rm.bytes)
		return
	}
	for i, m := range rm.members {
		if m != ss {
			continue
		}
		node := uint16(i + 1)
		rm.members[i] = nil
		rm.net.Bitmask &^= 1 << i
		rm.net.Nodes[i] = [NodeInfoSize]byte{}
		rm.setNodes()
		s.log.Info("node left", "network", hex32(rm.id()), "node", node, "reason", reasonName(reason), "nodes", rm.total())
		s.broadcastEvent(rm, nil, NodeEvent{Kind: EventLeft, Reason: reason, NetworkID: rm.id(), Node: node, Bitmask: rm.net.Bitmask, TotalNodes: rm.total()})
	}
}

func reasonName(r uint8) string {
	switch r {
	case ReasonClean:
		return "clean"
	case ReasonTimeout:
		return "timeout"
	case ReasonEjected:
		return "ejected"
	}
	return "unknown"
}

// broadcastEvent queues e for every member and spectator of rm except skip.
func (s *Server) broadcastEvent(rm *room, skip *session, e NodeEvent) {
	for _, m := range rm.members {
		if m != nil && m != skip {
			s.queueEvent(m, e)
		}
	}
	for sp := range rm.specs {
		if sp != skip {
			s.queueEvent(sp, e)
		}
	}
}

func (s *Server) queueEvent(ss *session, e NodeEvent) {
	ss.evQueue = append(ss.evQueue, appendNodeEvent(nil, e))
	if ss.inflight == nil {
		s.sendNextEvent(ss)
	}
}

func (s *Server) sendNextEvent(ss *session) {
	if len(ss.evQueue) == 0 {
		return
	}
	p := ss.evQueue[0]
	ss.evQueue = ss.evQueue[1:]
	ss.evSeq++
	ss.inflSeq = ss.evSeq
	ss.inflight = Packet(Header{Type: MsgNodeEvent, Session: ss.id, Seq: ss.evSeq}, p)
	ss.inflSent = time.Now()
	s.stats.EventsSent++
	s.write(ss, ss.inflight)
}

func (s *Server) data(ss *session, p []byte) {
	d, err := parseData(p)
	rm := ss.room
	if err != nil || rm == nil || ss.node == 0 {
		s.stats.DataDropped++
		return
	}
	s.stats.DataIn++
	d.Src = ss.node
	out := appendData(make([]byte, 0, len(p)), d)
	deliver := func(to *session) {
		to.outSeq++
		s.write(to, Packet(Header{Type: MsgData, Session: to.id, Seq: to.outSeq}, out))
		s.stats.DataOut++
		rm.packets++
		rm.bytes += uint64(len(d.Payload))
	}
	if d.Dst == BroadcastNode {
		for _, m := range rm.members {
			if m != nil && m != ss {
				deliver(m)
			}
		}
		for sp := range rm.specs {
			deliver(sp)
		}
		return
	}
	if d.Dst == 0 || d.Dst > MaxNodes || rm.members[d.Dst-1] == nil || d.Dst == ss.node {
		s.stats.DataDropped++
		return
	}
	deliver(rm.members[d.Dst-1])
}

func hex32(v uint32) string {
	const digits = "0123456789abcdef"
	b := []byte("0x00000000")
	for i := 9; i >= 2; i-- {
		b[i] = digits[v&0xF]
		v >>= 4
	}
	return string(b)
}

// equalAddr is used by the client to ignore datagrams from anyone but its server.
func equalAddr(a, b net.Addr) bool {
	ua, ok1 := a.(*net.UDPAddr)
	ub, ok2 := b.(*net.UDPAddr)
	if ok1 && ok2 {
		return ua.Port == ub.Port && ua.IP.Equal(ub.IP)
	}
	return a.String() == b.String()
}
