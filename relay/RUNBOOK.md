# Phase 3 runbook: relay server and fake consoles

The relay is a UDP server that both consoles dial outbound. It keeps rooms (hosted networks), answers scans, assigns node IDs on join, forwards data frames byte for byte, and tells the remaining nodes when one leaves, is ejected or goes silent. `udsfake` plays one or two consoles against it without hardware: either a scripted trade, or a replay of a trace recorded in Phase 1 or 2.

| Path | What |
| --- | --- |
| `cmd/udsrelay` | The relay server |
| `cmd/udsfake` | Fake consoles: `host`, `join`, `replay` |
| `internal/relay` | Wire protocol, server, client library, loss/latency wrapper |
| `internal/udsfake` | Fake console (a Go model of the shim's UDS state machine), trace replay, the Phase 3 exit test |

## Test

```bash
go test ./...
```

`TestPhase3Exit` (in `internal/udsfake/exit_test.go`) is the Phase 3 exit criterion. Two fake consoles host, scan, join, exchange data both ways and as broadcast at X/Y sizes, and see leave events: a clean disconnect, a third console that vanishes and is timed out, and the host destroying the network. `TestReplayTraces` replays all eight two-console traces in `recon/traces/`, including the hardware battle and the relay-mode runs on one LAN (`relay-hw-01`) and over the internet (`internet-hw-01`). `TestReplayTracesShim` replays the same traces through the shim core's C emulator instead of the Go model (it builds `shim/core/build/uds_replay`, so it needs a C compiler). `TestReplayWithLatency` replays the hardware battle with 40–60 ms of added latency and takes about 8 s; `go test -short ./...` skips it.

## Run the server

Every lobby has a key that the relay and each console in it share; nobody without it can use the relay (see Authentication below). Make one:

```bash
go run ./cmd/udsrelay -genkey
```

Put it in a lobbies file, one `name key` per line (`#` starts a comment), and keep that file private:

```text
home 3q2-hEXAMPLEkeyOnlyDoNotUse0aBcD
```

```bash
go run ./cmd/udsrelay -listen :28300 -lobbies lobbies.txt
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen` | `:28300` | UDP address. Open this UDP port on the firewall; consoles need nothing opened. |
| `-lobbies` | none | Lobbies file. The server refuses to start without at least one lobby. |
| `-lobby` | none | `name=key`, repeatable, for local tests. Keys given this way show up in `ps`. |
| `-timeout` | `5s` | A console silent this long is dropped and its peers get a node-left event. |
| `-keepalive` | `1s` | Keepalive interval the server tells consoles to use. |
| `-stats` | `1m` | Log counters this often. |
| `-v` | off | Debug logging, including every network PSS opens and closes in the background. |

At the default level the log shows sessions, joins, leaves (with the reason: clean, timeout or ejected), timeouts, and networks that carried traffic when they close.

## Run fake consoles

Three terminals: a local server with a throwaway lobby, then the two consoles using its key.

```bash
go run ./cmd/udsrelay -listen 127.0.0.1:28300 -lobby test=test-key
```

```bash
go run ./cmd/udsfake host -relay 127.0.0.1:28300 -lobby test-key
```

```bash
go run ./cmd/udsfake join -relay 127.0.0.1:28300 -lobby test-key
```

`-lobby` on `udsfake` is the key, not the name. A console with a key the relay does not know gets no answer and fails with "HELLO: relay did not answer".

The host opens a trade network as X/Y does (comm ID `0x00055d10`, ID8 `0x01`, 4 nodes) and waits. The joiner scans, joins as node 2, and both send `-packets` self-checking frames on channel 243 at `-rate` frames per second (default 20 at 3.5/s, X/Y's rate) and check what they receive byte for byte. Then the joiner leaves, and the host prints how long the leave took to reach it. Each exits non-zero on a missing or corrupt frame.

To test a remote relay, point both at it with `-relay host:28300`. To see the timeout path, kill the joiner with Ctrl-C mid-exchange: the host reports node 2 leaving about `-timeout` later. `-loss 0.05` and `-delay 80ms` degrade a console's own link (delay gets 25% jitter).

## Replay a trace

```bash
go run ./cmd/udsfake replay -relay 127.0.0.1:28300 host=recon/traces/battle-hw-01/console-host.log client=recon/traces/battle-hw-01/console-client.log
```

Each log's `nwm::UDS` calls run in order against a fake console, one goroutine per console. Every reply is compared with the recording, and every frame a console pulls must equal a frame the peer sent. Steps that depend on the peer wait for it (up to `-wait`): a scan before a join retries until the host's network appears, a pull the recording says got a frame waits for one, and a connection-status read waits for the node bitmask the recording shows. The report lists mismatches, and the command exits non-zero if there are any.

To replay across two machines, give both the same two logs and lobby and split the roles with `-only host` on one and `-only client` on the other. `-pace 1` replays at recorded speed instead of as fast as possible. The Azahar traces use `host.log` and `client.log`; in `drop-01` the instance labelled host is the one that joins.

Known Azahar-to-hardware differences are mapped to hardware before comparing: SetProbeResponseParam and 0x0023 succeed, and DestroyNetwork with no network fails with `0xC8A113EA`. A not-connected status compares the status only, since the reason and the leftover node ID differ between Azahar and hardware.

## Behaviour the fake console models (from the traces)

- **Results.** SetProbeResponseParam: success. 0x0023: success plus one word, `0x00881fff`. DestroyNetwork with no network: `0xC8A113EA`. PullPacket after disconnect or destroy: `0xC8A113EA`. Unbind of an unknown bind: success.
- **Status.** Hosting: ConnectedAsHost, node 1, bitmask 1, changed 1. Joining: ConnectedAsClient, the node ID the relay assigned, changed equal to the whole bitmask. A node joining or leaving sets its bit in the changed mask on the others, and reading the status clears it (hardware read changed `0x0003`, then `0x0000`). After DisconnectNetwork or DestroyNetwork: NotConnected with reason 2, keeping the old node ID. When the host goes away, a client gets NotConnected with reason ConnectionLost (4). No trace shows this case, so 4 is an assumption to check on hardware in Phase 4.
- **Network ID.** The relay gives each hosted network a random non-zero ID and writes it into the network struct at 0x18, big-endian. Clients join by that ID.
- **Channel.** GetChannel returns 11 while in a network, as hardware did.

## Rooms

Rooms live in a lobby (the key from the console's `config.ini`; consoles in different lobbies never see each other). A scan matches the wlan comm ID and ID8, with `0xFFFFFFFF` and `0xFF` as wildcards, and never returns the scanning console's own network. A join names the network ID and must carry the same SHA-256 of the passphrase the host registered. The relay never sees the passphrase itself. Pokémon X and Y share comm ID `0x00055d10`, so they meet in the same rooms.

Several networks can share comm ID and ID8 in one lobby. In PSS both consoles host ID8 `0x02` (2 nodes) at once and scan for each other's. A trade or battle is ID8 `0x01` with up to 4 nodes. A joiner gets the lowest free node ID from 2 up, and node IDs do not change when others leave.

The 5 s timeout is the design doc's figure. It is far inside the game's own 23–28 s peer timeout, so the survivor sees a node-left status change before it gives up on its own. It is also well above the game's longest idle gap (about 2.5 s), though keepalives make that irrelevant: they arrive every second whatever the game does.

## Wire protocol

UDP, big-endian, version 2. Every datagram starts with a 12-byte header and ends with a 16-byte authentication tag (see Authentication below):

| Offset | Size | Field |
| --- | --- | --- |
| 0 | 2 | magic `UR` |
| 2 | 1 | version (2) |
| 3 | 1 | message type |
| 4 | 4 | session ID, chosen at random by the console, non-zero |
| 8 | 2 | sequence number |
| 10 | 2 | ack: the seq of the request this answers, or of the NODE_EVENT an ACK acknowledges |

The console retries requests every 250 ms until the response arrives (3 s limit). The server caches its last response per session, so a retried request gets the same answer and is not applied twice. NODE_EVENT is the one reliable message from the server: one at a time per session, resent every 200 ms until the console ACKs it. DATA and KEEPALIVE are unreliable, like UDS itself. A datagram from an unknown session gets RESET, after which the console says HELLO again and treats its network as lost (server restart).

| Type | Name | Direction | Payload |
| --- | --- | --- | --- |
| 1 | HELLO | → relay | u32 lobby ID: the first 4 bytes of SHA-256(`"udsrelay lobby id\0"` + key). The key itself is never sent. |
| 2 | HELLO_ACK | ← relay | status, u16 keepalive ms, u16 timeout ms |
| 3 | HOST_CREATE | → relay | SHA-256(passphrase) [32], host NodeInfo [0x28], network struct [0x108] |
| 4 | HOST_ACK | ← relay | status, u32 network ID |
| 5 | HOST_UPDATE | → relay | u8 kind: 1 = app data (u8 length, ≤ 0xC8 bytes), 2 = attributes (u16) |
| 6 | UPDATE_ACK | ← relay | status |
| 7 | SCAN_REQ | → relay | u32 comm ID, u8 ID8 |
| 8 | SCAN_RESP | ← relay | status, u8 index, u8 total, network. One datagram per network, so a lost one just drops that network from the scan. Total 0: nothing found. |
| 9 | JOIN_REQ | → relay | u32 network ID, SHA-256(passphrase) [32], u8 conn type (1 client, 2 spectator), NodeInfo [0x28] |
| 10 | JOIN_ACK | ← relay | status, u16 node ID (0 for a spectator), network |
| 11 | NODE_EVENT | ← relay | u8 kind (1 joined, 2 left, 3 ejected you, 4 network closed), u8 reason (0 clean, 1 timeout, 2 ejected), u32 network ID (ignore events for a network you have left), u16 node, u16 bitmask, u8 total nodes, NodeInfo [0x28] (joined only) |
| 12 | DATA | ↔ | u16 src node, u16 dest node (0xFFFF broadcast), u8 channel, u8 flags, payload |
| 13 | EJECT | → relay | u16 node; 0 ejects all spectators. Host only. |
| 14 | EJECT_ACK | ← relay | status |
| 15 | LEAVE | → relay | none. A leaving host closes the network. |
| 16 | LEAVE_ACK | ← relay | status |
| 17 | KEEPALIVE | → relay | u64 send time (ns), echoed for RTT |
| 18 | KEEPALIVE_ACK | ← relay | the same u64 |
| 19 | ACK | → relay | none; header ack = the NODE_EVENT's seq |
| 20 | RESET | ← relay | none |

"Network" is the network struct [0x108] with the relay's network ID and node count filled in, then u16 node bitmask and one NodeInfo [0x28] per set bit. Status codes: 0 OK, 1 not found, 2 full, 3 passphrase mismatch, 4 bad request, 5 not the host.

The relay overwrites a DATA frame's source with the sender's node ID. It routes to one node, or to every other member and spectator for 0xFFFF; spectators cannot send. Overhead is 18 bytes of relay header plus 28 of IP and UDP, so X/Y's largest frame (1349 bytes) makes a 1395-byte IP packet. Payloads up to 1454 bytes fit a 1500-byte MTU, and anything larger, up to UDS's 0x5C6 limit, relies on IP fragmentation; Phase 5 should measure whether that hurts.

## Authentication

The design doc's v1 choice, kept deliberately small: every datagram ends with the first 16 bytes of HMAC-SHA256 over the rest of the datagram (header and payload), keyed by the lobby key. Both directions are tagged.

- The relay only serves lobbies from its lobbies file. A HELLO names its lobby by lobby ID; the relay verifies the tag with that lobby's key and drops the datagram if it does not match. A datagram from a known session is checked against that session's key.
- Datagrams without a valid tag get no answer at all and count as `BadTag` in the stats, so strangers cannot use the relay or make it send traffic to a spoofed address. A console whose session the relay lost (restart or timeout) still gets RESET, because its tag verifies against one of the configured keys.
- Consoles drop anything from the relay whose tag does not verify.
- A session's address only changes when a datagram arrives with a newer sequence number than any seen before. A replayed old datagram from somewhere else is answered to the session's real address and cannot redirect its traffic; a genuine NAT rebinding moves the session with the next keepalive.

What it does not do: encrypt anything, or reject replays outright. A replayed request gets the cached answer (sent to the real console), and a replayed DATA frame is delivered again, which the game sees as a duplicate packet. Closing those gaps means a handshake with per-session keys and an AEAD; the design doc records this as a later step.

## Not done yet

- **Authentication only, not encryption.** Traffic, including each console's NodeInfo (username, friend-code seed) and game data, crosses the internet in the clear. A captured datagram can be replayed; see Authentication.
- **No reliable data mode.** The design doc's optional per-channel reliable mode is not built. X/Y's hardware battle completed with 3–4% of frames unreceived, so plain UDP may be enough for it.
- **Join failure result.** No trace shows what NWM returns when ConnectToNetwork fails; the fake returns `0xC8A113EA` as a placeholder.
- **0x0023's word.** It is not constant on hardware. battle-hw-01 shows `0xebee1fff`, `0x16c31fff`, `0xd3841fff` and `0x36791fff`, one per initialisation; only the low half (`0x1fff`) repeats. The fake returns `0x00881fff` as the design doc says, and the replay compares only the low half.
- **Leftover sessions.** A console that leaves stays a session until it goes quiet for the timeout; there is no goodbye message.
- **Beacons.** Scans return network structs and node lists, not 802.11 beacons. The Phase 4 shim synthesises the beacons and answers DecryptBeaconData (`shim/core/src/uds.c`).
