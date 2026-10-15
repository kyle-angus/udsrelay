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
func Build(recs []Record) []Item {
	var items []Item
	var open *Call // request whose immediate reply has not been logged yet
	var calls []*Call
	for i := range recs {
		r := &recs[i]
		switch r.Kind {
		case "req":
			c := &Call{Instance: r.Instance, Port: r.Port, Cmd: r.Cmd, Name: r.Name, Proc: r.Proc, Req: r}
			if c.Name == "" {
				c.Name = CommandName(r.Port, r.Cmd)
			}
			open = c
			calls = append(calls, c)
			items = append(items, Item{Wall: r.Wall, Instance: r.Instance, Call: c})
		case "blob":
			if open != nil && open.Cmd == r.Cmd {
				open.Blobs = append(open.Blobs, r)
			} else if c := lastCall(calls, r.Port, r.Cmd); c != nil {
				c.Blobs = append(c.Blobs, r)
			}
		case "unimpl":
			if open != nil && open.Cmd == r.Cmd {
				open.Unimplemented = true
				open = nil
			}
		case "rsp":
			if open == nil || open.Cmd != r.Cmd || open.Port != r.Port {
				continue
			}
			if r.Hdr == open.Req.Hdr {
				// Header unchanged: the handler deferred its reply to a wake-up callback.
				open.AwaitingWake = true
			} else {
				open.Reply = r.Words
			}
			open = nil
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

func lastCall(calls []*Call, port string, cmd uint16) *Call {
	for j := len(calls) - 1; j >= 0; j-- {
		if calls[j].Port == port && calls[j].Cmd == cmd {
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
