# Phase 1 recon runbook: Pokémon X/Y UDS trace in Azahar

Goal (design doc, Phase 1 exit): a written command trace for Pokémon X/Y, covering which `nwm::UDS` commands and events the game depends on for a local trade and a local battle. The trace should also confirm whether X and Y share one wlan comm ID, and show what PSS does with UDS in the background.

How it works: two Azahar instances on this Mac join one local network room. A small patch makes Azahar log every `nwm::UDS`, `ndm:u` and `ac:u`/`ac:i` IPC call, plus each reply, event signal and payload, with wall-clock timestamps. `udstrace` then merges the two logs into one timeline and a Markdown summary.

## 1. Build patched Azahar

Run these commands from this repository's root.

The patch targets Azahar commit in [`azahar/BASE_COMMIT`](azahar/BASE_COMMIT). Other commits will probably work, but `git apply` may need `--3way`.

```bash
git clone --recursive https://github.com/azahar-emu/azahar.git ~/src/azahar
```

```bash
git -C ~/src/azahar checkout 68202bc4ff7edf9d7280ec0dcd6572b7cd1ae26e
```

```bash
git -C ~/src/azahar submodule update --init --recursive
```

```bash
git -C ~/src/azahar apply "$PWD/recon/azahar/udstrace.patch"
```

Build it by following Azahar's [Building From Source](https://github.com/azahar-emu/azahar/wiki/Building-From-Source) wiki page for macOS. Add `-DCITRA_WARNINGS_AS_ERRORS=OFF` to the CMake configure line: Azahar treats warnings as errors by default, and this patch has not yet been compiled inside the full Azahar tree.

Point the helper at the binary inside the bundle, not the `.app` itself:

```bash
export AZAHAR=~/src/azahar/build/bin/Release/azahar.app/Contents/MacOS/azahar
```

Check the exact path in your build output; it differs between generators.

## 2. What the patch logs

Every line contains `UDSTRACE v=1` and logs at Info level, so Azahar's default `*:Info` log filter keeps it. Kinds:

| Kind | When | Carries |
| --- | --- | --- |
| `req` | A traced port receives a request | command ID and name, calling thread and process, raw request words |
| `rsp` | The handler returns | raw reply words; `deferred=1` if it slept the client thread |
| `wake` | ConnectToNetwork's deferred reply is written | result words |
| `unimpl` | Azahar has no handler for the command | header |
| `signal` | NWM signals an event | event name (`NWM::connection_status_event`, `NWM::BindNodeEvent<n>`) |
| `blob` | Payloads the words don't carry | `netinfo` (0x108-byte network struct), `passphrase`, `appdata`, `send`, `recv` |

Environment knobs, read once at startup:

- `UDSTRACE_PORTS`: comma-separated ports to trace. The default is `nwm::UDS,ndm:u,ac:u,ac:i`. Set it to an empty string to disable tracing.
- `UDSTRACE_PAYLOAD_MAX`: bytes of each send/recv payload to log. The default is `512`; `-1` logs all of it. Azahar stops writing a log at 100 MiB, and full payloads with polling reach that in roughly an hour of play.

For Azahar's own NWM/NDM debug lines as well, set the log filter (Emulation → Configure → Debug) to `*:Info Service.NWM:Debug Service.NDM:Debug`. `udstrace` shows them as notes.

## 3. Set up two instances

```bash
recon/recon.sh setup
```

This creates `recon/run/host/user` and `recon/run/client/user`, which are gitignored. When Azahar's working directory contains a `user/` folder, it uses that folder for config, saves and logs, so the two instances never share a log file.

Launch each instance once (`recon/recon.sh launch host`, then `client`) and set it up:

- Give each one a different system username (System settings) so the trace can tell them apart. The username appears in `InitializeWithVersion`.
- Install or point each one at its own copy of the game, with a save that is past the point where local trading and battling unlock.
- Copy in any keys or system files your setup already uses, since each user dir starts empty.

## 4. Join both instances to one room

1. In the **host** instance: Multiplayer → Host Room. Pick any port, leave it unlisted, and give it a nickname.
2. In the **client** instance: Multiplayer → Direct Connect to `127.0.0.1` on that port, with a different nickname.

Both games now see each other as if they were on local wireless.

## 5. Capture sessions

For every session: launch both instances (`recon/recon.sh launch host` and `recon/recon.sh launch client` in two terminals), join the room, play the scenario, quit both emulators cleanly, then collect:

```bash
recon/recon.sh collect trade-01
```

Collect before the next launch. Azahar renames the previous log to `azahar_log.old.txt` at startup, and overwrites that file on the launch after.

| Session | Scenario | Answers |
| --- | --- | --- |
| `idle-01` | Both games in the overworld for 2 minutes with PSS open. No trade or battle. | What PSS does in the background: beacons, scans, any connects. This is the baseline to subtract from the other sessions. |
| `trade-01` | Host starts a local trade from PSS, trades one Pokémon each way, both back out. | Host, scan, join, data and leave sequence for a trade. |
| `battle-01` | One local single battle to completion. | Whether battles use different channels, flags, packet rates or sizes. |
| `xy-01` | Trade between X (host) and Y (client), if you have both. | Whether X and Y share one wlan comm ID and passphrase. Compare the Network parameters sections. |
| `drop-01` | Start a trade, then quit the client emulator mid-trade. | Leave and timeout semantics: what the host sees when a node disappears. |

## 6. Read the output

Each `recon/traces/<session>/` folder holds the two raw logs and:

- `timeline.txt`: one merged timeline across both instances. Unchanged polls, data packets and per-packet events fold into `·` lines with counts, so the protocol flow stays readable.
- `timeline-verbose.txt`: every call, with the first 16 payload bytes of each send and receive.
- `summary.md`: sessions, milestones (init, host, scan, join, data, leave), decoded network parameters, command usage per instance (flagging commands missing from the design doc's emulation table), failures, events, data-channel stats, and NDM/AC calls.

Run the tool directly for other combinations:

```bash
go run ./cmd/udstrace timeline -v host=recon/traces/trade-01/host.log client=recon/traces/trade-01/client.log
```

## 7. Done when

- `trade-01` and `battle-01` summaries both show every milestone, with no unexplained failures.
- Every command in the summaries marked **no** under Planned is either added to the design doc's emulation table or noted as safe to stub.
- The comm ID question is answered (`xy-01`), and the PSS background behaviour is described from `idle-01`.

Write those findings into the design doc's Phase 1 entry and its open questions.

## Known limits

- The full Azahar build with this patch has not been compiled yet; only the new `udstrace.h` header was compile-checked on its own. Expect possible small build fixes on first use.
- This traces Azahar's HLE NWM, which is the behavioural reference, not real hardware. Phase 2's on-console passthrough logs are what confirm real NWM behaviour.
- Azahar handles at most one bind per data channel and stubs a few commands (EjectClient, EjectSpectators). If the game behaves differently in Azahar than on hardware, those stubs are the first suspects.
