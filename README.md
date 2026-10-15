# 3DS Local Wireless Relay

Local-wireless multiplayer for real 3DS consoles over the internet. A shim on each console answers the game's `nwm::UDS` calls and relays the traffic through a small server. The design, architecture and phased plan are in the [design doc](https://claude.ai/code/artifact/f58bd746-9f54-4357-8924-2776589a39df).

## Status

The first target is Pokémon X/Y: a local trade, then a local battle.

- Phase 1 (recon in Azahar): done. Traces for idle, trade, battle, two drop cases and an X–Y trade are in `recon/traces/`; findings are in the design doc's Phase 1 entry.
- Phase 2 (passthrough shim on hardware): done. A full trade and a full battle ran between two consoles through the shim; the battle trace is in `recon/traces/battle-hw-01/`, and the differences from Azahar are in the design doc.
- Phase 3 (relay server): done. `udsrelay` serves rooms, scan, join, node events, data forwarding and keepalive over UDP. `udsfake` fake consoles pass the exit test and replay all six two-console traces through it with hardware's result codes and byte-for-byte data.
- Phase 4 (relay mode): done on one LAN. A full trade and a full battle ran between two consoles through the relay (`shim/RUNBOOK.md` section 8). The shim core emulates `nwm::UDS` over the relay (`shim/core/src/uds.c`), and the plugin runs it with `mode = relay` in `shim.ini`. All six traces replay through the C emulator against the relay (`TestReplayTracesShim`).

## Layout

| Path | What |
| --- | --- |
| `recon/RUNBOOK.md` | How to capture a Pokémon X/Y UDS trace with two Azahar instances |
| `recon/azahar/udstrace.patch` | Azahar patch that logs every `nwm::UDS` / `ndm:u` / `ac` IPC call, reply, event and payload |
| `recon/recon.sh` | Sets up per-instance user dirs, launches instances, collects logs into `recon/traces/` |
| `cmd/udstrace` | Turns the logs into a merged timeline and a Markdown command-trace summary |
| `internal/udstrace` | Parser, request/reply pairing, UDS struct decoders and trace comparison, with tests on synthetic fixtures |
| `shim/RUNBOOK.md` | How to build the Phase 2 shim, install it on two consoles, and compare its logs against the Azahar trace |
| `shim/core` | Host-agnostic C: IPC parsing, passthrough forwarding loop, GetServiceHandle finder, trace formatting, the relay wire protocol and client, and the `nwm::UDS` emulator with beacon synthesis for relay mode |
| `shim/plugin` | CTRPluginFramework 3GX plugin: hooks GetServiceHandle, runs the session threads (forwarding, or the emulator in relay mode) and the relay thread, writes logs to SD |
| `relay/RUNBOOK.md` | How to run the relay and fake consoles, replay traces, and the wire protocol |
| `cmd/udsrelay` | The relay server |
| `cmd/udsfake` | Fake consoles: a scripted host and joiner, and trace replay |
| `internal/relay` | Wire protocol, server, client library, and a loss/latency wrapper for tests |
| `internal/udsfake` | Fake console modelling the shim's UDS state machine, trace replay (through the Go model or the shim core's C emulator), and the Phase 3 exit test |

## Develop

```bash
go test ./...
```

```bash
make -C shim/core test
```

```bash
go run ./cmd/udsrelay -lobby test=test-key
```

```bash
go run ./cmd/udstrace summary host=internal/udstrace/testdata/host.log client=internal/udstrace/testdata/client.log
```

The fixtures in `internal/udstrace/testdata` are synthetic, not real captures. `host.log` and `client.log` come from `gen_fixtures.py` in the Azahar patch's line format. `console_host.log` comes from the shim core's fake-kernel test (`make -C shim/core fixtures`), so the Go tests also check that the C and Go sides agree on the format.
