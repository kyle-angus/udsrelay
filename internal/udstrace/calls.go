package udstrace

import (
	"sort"
)

// Call is one IPC request with its reply, payloads and (for sleeping commands) wake-up.
type Call struct {
	Instance      string
	Port          string
	Cmd           uint16
	Name          string
	Proc          string
	Req           *Record
	Blobs         []*Record
	Unimplemented bool
	// AwaitingWake is set when the handler slept the client thread without writing a reply
	// (ConnectToNetwork); the reply then comes from the wake record.
	AwaitingWake bool
	Wake         *Record
	// Reply holds the reply words including the header, or nil if never seen.
	Reply []uint32
}

// Result returns the reply's result code, if a reply was seen.
func (c *Call) Result() (uint32, bool) {
	if len(c.Reply) < 2 {
		return 0, false
	}
	return c.Reply[1], true
}

// Blob returns the first payload with the given tag.
func (c *Call) Blob(tag string) *Record {
	for _, b := range c.Blobs {
		if b.Tag == tag {
			return b
		}
	}
	return nil
}

// Item is one timeline entry: a call, an event signal, or a stock log note.
type Item struct {
	Wall     uint64
	Instance string
	Call     *Call
	Rec      *Record
}

// Build pairs one instance's records into calls and standalone items, in log order.
//
// Requests still waiting for their reply stay open, and each reply, payload or unimpl record
// goes to the newest open request with the same port, command and session. In Azahar a reply
// always follows its request, but the console shim forwards several sessions at once (X/Y keeps
// two nwm::UDS sessions busy during a trade), so their lines interleave.
func Build(recs []Record) []Item {
	var items []Item
	var open []*Call // requests whose reply has not been logged yet
	var calls []*Call
	takeOpen := func(r *Record) *Call {
		for j := len(open) - 1; j >= 0; j-- {
			if c := open[j]; sameCall(c, r) {
				open = append(open[:j], open[j+1:]...)
				return c
			}
		}
		return nil
	}
	for i := range recs {
		r := &recs[i]
		switch r.Kind {
		case "req":
			c := &Call{Instance: r.Instance, Port: r.Port, Cmd: r.Cmd, Name: r.Name, Proc: r.Proc, Req: r}
			if c.Name == "" {
				c.Name = CommandName(r.Port, r.Cmd)
			}
			open = append(open, c)
			calls = append(calls, c)
			items = append(items, Item{Wall: r.Wall, Instance: r.Instance, Call: c})
		case "blob":
			if c := lastCall(calls, r); c != nil {
				c.Blobs = append(c.Blobs, r)
			}
		case "unimpl":
			if c := takeOpen(r); c != nil {
				c.Unimplemented = true
			}
		case "rsp":
			c := takeOpen(r)
			if c == nil {
				continue
			}
			if r.Deferred && r.Hdr == c.Req.Hdr {
				// Deferred with the header unchanged: the handler left its reply to a wake-up
				// callback. Without deferred=1 an unchanged header is just a reply shaped like
				// the request, such as ndm:u's one-word-in, one-word-out commands.
				c.AwaitingWake = true
			} else {
				c.Reply = r.Words
			}
		case "wake":
			for j := len(calls) - 1; j >= 0; j-- {
				c := calls[j]
				if c.AwaitingWake && c.Wake == nil && c.Cmd == r.Cmd && c.Port == r.Port {
					c.Wake, c.Reply = r, r.Words
					break
				}
			}
		default: // signal, note
			items = append(items, Item{Wall: r.Wall, Instance: r.Instance, Rec: r})
		}
	}
	return items
}

// sameCall reports whether record r (a reply, payload or unimpl) can belong to call c.
func sameCall(c *Call, r *Record) bool {
	return c.Port == r.Port && c.Cmd == r.Cmd && c.Req.Sess == r.Sess
}

func lastCall(calls []*Call, r *Record) *Call {
	for j := len(calls) - 1; j >= 0; j-- {
		if sameCall(calls[j], r) {
			return calls[j]
		}
	}
	return nil
}

// Merge combines per-instance items into one timeline ordered by wall clock.
// Items with equal wall time keep their per-instance log order.
func Merge(perInstance ...[]Item) []Item {
	var all []Item
	for _, items := range perInstance {
		all = append(all, items...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Wall < all[j].Wall })
	return all
}
