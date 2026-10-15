# 3DS Local Wireless Relay

Local-wireless multiplayer for real 3DS consoles over the internet. A shim on each console answers the game's `nwm::UDS` calls and relays the traffic through a small server. The design, architecture and phased plan are in the [design doc](https://claude.ai/code/artifact/f58bd746-9f54-4357-8924-2776589a39df).

## Status

The first target is Pokémon X/Y: a local trade, then a local battle.

- Phase 1 (recon in Azahar): tooling is in place; no real trace has been captured yet.
- Phase 2 (passthrough shim on hardware): the 3GX plugin is written. Its core is tested on a PC against a fake kernel; the plugin itself has not been compiled or run on a console yet.

## Layout

| Path | What |
| --- | --- |
| `recon/RUNBOOK.md` | How to capture a Pokémon X/Y UDS trace with two Azahar instances |
| `recon/azahar/udstrace.patch` | Azahar patch that logs every `nwm::UDS` / `ndm:u` / `ac` IPC call, reply, event and payload |
| `recon/recon.sh` | Sets up per-instance user dirs, launches instances, collects logs into `recon/traces/` |
| `cmd/udstrace` | Turns the logs into a merged timeline and a Markdown command-trace summary |
| `internal/udstrace` | Parser, request/reply pairing, UDS struct decoders and trace comparison, with tests on synthetic fixtures |
| `shim/RUNBOOK.md` | How to build the Phase 2 shim, install it on two consoles, and compare its logs against the Azahar trace |
| `shim/core` | Host-agnostic C: IPC parsing, passthrough forwarding loop, GetServiceHandle finder, trace formatting |
| `shim/plugin` | CTRPluginFramework 3GX plugin: hooks GetServiceHandle, runs the forwarding threads, writes logs to SD |

## Develop

```bash
go test ./...
```

```bash
make -C shim/core test
```

```bash
go run ./cmd/udstrace summary host=internal/udstrace/testdata/host.log client=internal/udstrace/testdata/client.log
```

The fixtures in `internal/udstrace/testdata` are synthetic, not real captures. `host.log` and `client.log` come from `gen_fixtures.py` in the Azahar patch's line format. `console_host.log` comes from the shim core's fake-kernel test (`make -C shim/core fixtures`), so the Go tests also check that the C and Go sides agree on the format.
