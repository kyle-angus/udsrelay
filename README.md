# 3DS Local Wireless Relay

Local-wireless multiplayer for real 3DS consoles over the internet. A shim on each console answers the game's `nwm::UDS` calls and relays the traffic through a small server. The design, architecture and phased plan are in the [design doc](https://claude.ai/code/artifact/f58bd746-9f54-4357-8924-2776589a39df).

## Status

Phase 1 (recon in Azahar) tooling is in place, and no real trace has been captured yet. The first target is Pokémon X/Y: a local trade, then a local battle.

## Layout

| Path | What |
| --- | --- |
| `recon/RUNBOOK.md` | How to capture a Pokémon X/Y UDS trace with two Azahar instances |
| `recon/azahar/udstrace.patch` | Azahar patch that logs every `nwm::UDS` / `ndm:u` / `ac` IPC call, reply, event and payload |
| `recon/recon.sh` | Sets up per-instance user dirs, launches instances, collects logs into `recon/traces/` |
| `cmd/udstrace` | Turns the logs into a merged timeline and a Markdown command-trace summary |
| `internal/udstrace` | Parser, request/reply pairing and UDS struct decoders, with tests on synthetic fixtures |

## Develop

```bash
go test ./...
```

```bash
go run ./cmd/udstrace summary host=internal/udstrace/testdata/host.log client=internal/udstrace/testdata/client.log
```

The fixtures in `internal/udstrace/testdata` are synthetic, produced by `gen_fixtures.py` in the patch's exact line format. They are not real captures.
