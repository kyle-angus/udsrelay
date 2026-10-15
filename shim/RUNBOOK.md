# Phase 2 runbook: passthrough shim on real consoles

Goal (design doc, Phase 2 exit): two real consoles play Pokémon X/Y locally with the shim in the path, and the shim's logs match the Phase 1 Azahar trace.

## How the shim works

The shim is a 3GX plugin (`shim/plugin`) built on CTRPluginFramework. Luma's plugin loader runs it inside the game process before the game's first instruction.

1. **Hook.** It scans the game's code for the SDK's `srv:GetServiceHandle` client function, found by the `0x00050100` request header it loads from a literal pool, and MITM-hooks it.
2. **Swap.** When that function returns a handle, the hook asks Luma for the handle's service name (`svcControlService(SERVICEOP_GET_NAME)`). For a traced service (`nwm::UDS` and `ndm:u` by default), it creates a session pair and gives the game the client end in place of the real handle.
3. **Forward.** A thread per session receives each request, forwards it unchanged to the real service, and returns the real reply. By the time a request arrives, the kernel has already translated its handles and buffers into this process, so the same command buffer is valid to send on. The same holds for the reply on the way back.
4. **Log.** Every request, reply and payload is written in the same `UDSTRACE v=1` format as the Azahar patch, so `udstrace` reads both.

Logs go to `sdmc:/udsrelay/logs/<titleid>-<UTC time>.log`, not over the network. In passthrough the real NWM owns the radio, and the game's `ndm:u` request for exclusive local-communications state really does drop infrastructure Wi-Fi, so there is no network during local play.

The logic lives in `shim/core` (host-agnostic C), tested on a PC against a fake kernel: `make -C shim/core test`. `shim/plugin` is the thin console glue.

## 1. Toolchain (macOS)

1. Install devkitPro pacman from the [devkitPro installer releases](https://github.com/devkitPro/pacman/releases) (the macOS `.pkg`), then install the 3DS toolchain:

   ```bash
   sudo dkp-pacman -S 3ds-dev
   ```

2. Add CTRPluginFramework's package database (its `lib` repository is architecture-independent) and install libctrpf:

   ```bash
   printf '\n[thepixellizeross-lib]\nServer = https://thepixellizeross.gitlab.io/packages/any\nSigLevel = Optional\n' | sudo tee -a /opt/devkitpro/pacman/etc/pacman.conf
   ```

   ```bash
   sudo dkp-pacman -Sy libctrpf
   ```

3. Build 3gxtool from source; its prebuilt packages are Windows and Linux only.

   ```bash
   brew install cmake
   ```

   ```bash
   git clone --recursive https://gitlab.com/thepixellizeross/3gxtool.git ~/src/3gxtool
   ```

   It does not build or run reliably on macOS as-is, so patch it first. The bundled dynalo library only recognises Linux and Windows, but its Linux backend is plain `dlopen`, so let macOS use it:

   ```bash
   sed -i '' 's/defined(_LINUX)$/defined(_LINUX) || defined(__APPLE__)/' ~/src/3gxtool/extern/dynalo/include/dynalo/detail/config.hpp
   ```

   [`shim/3gxtool-macos.patch`](3gxtool-macos.patch) limits the `-static` link flags (which macOS does not support) to other platforms, and fixes the symbol-sort comparator. As shipped, it can say two `$` mapping symbols at the same address each sort before the other; macOS's libc++ then reads past the array and 3gxtool segfaults on some ELFs. From this repository's root:

   ```bash
   git -C ~/src/3gxtool apply "$PWD/shim/3gxtool-macos.patch"
   ```

   ```bash
   cmake -S ~/src/3gxtool -B ~/src/3gxtool/build -DCMAKE_POLICY_VERSION_MINIMUM=3.5 && cmake --build ~/src/3gxtool/build
   ```

   The policy flag is needed because the bundled yaml-cpp declares a CMake minimum below 3.5, which CMake 4 rejects.

   Install the resulting binary in `/opt/devkitpro/tools/bin`. devkitPro's make rules put that folder first on `PATH`, so a copy anywhere else is ignored during the build.

   ```bash
   sudo cp ~/src/3gxtool/build/3gxtool /opt/devkitpro/tools/bin/
   ```

4. Set the environment (add to your shell profile):

   ```bash
   export DEVKITPRO=/opt/devkitpro DEVKITARM=/opt/devkitpro/devkitARM
   ```

## 2. Build

```bash
shim/plugin/build.sh
```

The build script copies `shim/core` and `shim/plugin` to `/tmp/udsrelay-shim-build` before running `make`, because devkitPro's make rules break on the spaces in this repository's path. The output is `shim/plugin/udsrelay-shim.3gx`.

## 3. Install on both consoles

1. Both consoles need Luma3DS with the plugin loader.
2. Copy `udsrelay-shim.3gx` to `sdmc:/luma/plugins/<title ID>/` on each console. Pokémon X is `0004000000055D00` and Y is `0004000000055E00`; confirm against your copy, for example in FBI's title list.
3. Optional: copy `shim/plugin/shim.ini.example` to `sdmc:/udsrelay/shim.ini` to change traced ports or payload logging. Add `ac:u` to the ports to see whether the game checks Wi-Fi state during local play.
4. Enable the plugin loader: open Rosalina (L + Down + Select) → Plugin Loader → Enabled.

## 4. Capture

1. Start the game on both consoles. When the game first opens a traced service, a notification appears: "UDS shim: N session(s) intercepted". X/Y's PSS opens `nwm::UDS` soon after the save loads.
2. Play the same scenarios as Phase 1 (`idle`, `trade`, `battle`; see `recon/RUNBOOK.md`), one session per game launch.
3. Check the plugin menu (Select by default) → Status: hooks installed, sessions intercepted, bytes written and dropped lines.
4. Quit the game from the HOME menu. The log flushes every second, so at most the last second is lost.
5. Copy the new log from `sdmc:/udsrelay/logs/` off each console (SD card reader, or ftpd) before the next session, under a name that includes the session, for example `trade-hw-02-host.log`. Two sessions copied to the same name overwrite each other.
6. Redact each log into `recon/traces/<session>/`. The game's first scan returns every nearby Wi-Fi access point, so the raw logs contain third-party network names and MAC addresses. They also carry your consoles' own MACs and friend-code seeds. `redact` empties every beacon payload, zeroes the host MAC in netinfo and the NodeInfo friend-code seeds (also where X/Y puts them in its own data frames during a trade), and keeps the rest:

   ```bash
   go run ./cmd/udstrace redact ~/Desktop/trade-hw-02-host.log recon/traces/trade-hw-02/console-host.log
   ```

   Do the same for the client log. Keep the raw logs out of the repository.

## 5. Compare against the Azahar trace

```bash
go run ./cmd/udstrace compare azahar=recon/traces/battle-01/host.log console=recon/traces/battle-hw-01/console-host.log
```

```bash
go run ./cmd/udstrace compare azahar=recon/traces/battle-01/client.log console=recon/traces/battle-hw-01/console-client.log
```

`compare` reports commands present in only one trace, commands whose result codes differ, changes in the order of significant calls (polls and data packets are left out of the order check), and network parameter differences. It exits 1 on any difference.

For a merged two-console timeline, line the clocks up with `-shift`. Console clocks can differ by anything from seconds to hours (about an hour in `battle-hw-01`), so pick an offset in milliseconds that puts the host's first GetConnectionStatus showing two nodes just after the client's ConnectToNetwork. For `battle-hw-01`:

```bash
go run ./cmd/udstrace timeline -shift host=-3591640 host=recon/traces/battle-hw-01/console-host.log client=recon/traces/battle-hw-01/console-client.log
```

Expected differences from Azahar, which are not shim bugs:

- Azahar replies to `ConnectToNetwork` asynchronously and logs `wake` lines. On hardware the reply comes back directly.
- Azahar stubs `EjectClient` and `EjectSpectators` and has no handler for some commands. Real NWM returns real results.
- Console logs have no `signal` lines: the shim cannot see NWM signal its events without stealing those signals from the game.
- Real NWM returns success for SetProbeResponseParam (`0x0021`) and for `0x0023` (plus one word, `0x00881fff`); Azahar has no handler for either.
- DestroyNetwork with no network up fails with `0xC8A113EA` on hardware and `0xC8A111EA` in Azahar.
- The network a client joins has a real network ID on hardware (random per hosting); Azahar uses 0.
- Many call-order changes: PSS's host/scan cycle timing is random, so the background cycles never line up. Check the order around Bind, BeginHostingNetwork (ID8 `0x01`), ConnectToNetwork and teardown instead.

Phase 2 captures: `battle-hw-01` (two consoles, a full local battle). The first hardware trade was overwritten on the SD card before it was copied; it matched `trade-01` with the same differences as above.

## 6. Done when

- A full trade and a full battle complete between two consoles with the shim active, with no dropped lines.
- `compare` shows no commands, results or call-order differences against the Azahar trace beyond the expected ones above, and each remaining difference is written up in the design doc.

## 7. Phase 4 relay check (one console is enough)

Before the shim emulates UDS over the relay, two things need confirming on hardware: that the plugin can open a socket from inside Pokémon X, and that infrastructure Wi-Fi survives the game entering local-wireless mode once the shim fakes NDM's exclusive state.

1. On a PC on the same network as the console, start the relay with a lobby and note the PC's LAN address (System Settings → Network on macOS). Allow incoming UDP 28300 if the firewall asks.

   ```bash
   go run ./cmd/udsrelay -genkey
   ```

   ```bash
   go run ./cmd/udsrelay -listen :28300 -lobby home=<the key>
   ```

2. On the console, write `sdmc:/udsrelay/shim.ini`:

   ```ini
   relay = <PC's LAN address>:28300
   lobby = <the key>
   fake_ndm = 0
   ```

3. Install the latest `udsrelay-shim.3gx`, make sure Wi-Fi is on (the HOME menu's Wi-Fi light), launch the game and load the save. Within a few seconds a notification says **relay reachable** or **relay unreachable: why**, and the relay's terminal logs a session. Wait 30 s on the overworld with PSS open, then quit.
4. Set `fake_ndm = 1`, launch again, wait 30 s with PSS open, then start a local trade search from PSS for 30 s. With the shim answering NDM, real local wireless may not work: an error from the game here is expected and still informative.
5. Copy both logs off. The `RelayCheck` lines show, every 5 s, whether HELLO got an answer, the Wi-Fi and AC state, and the real NDM exclusive state next to the one the game believes; the `ndm:u` trace lines show when the game asked for local mode.

What the results mean:

| Result | Meaning |
| --- | --- |
| `socInit failed 0xe0a01bf5` (module 6) | The kernel refused the socket buffer's address. Fixed by allocating it from the plugin heap; seeing it again means the heap is in an unexpected place. |
| `socInit failed` with another module | Most likely srv refusing `soc:U` because the game's service access list lacks it. Relay mode would then need a sysmodule outside the game process. |
| HELLO fails from the start, Wi-Fi 0 | Sockets work but the console is not connected; the shim will have to bring up AC itself before relaying. |
| HELLO ok at the title screen, fails once PSS starts, `fake_ndm = 0` | The game's EnterExclusiveState drops Wi-Fi, as expected; this is why NDM must be faked. |
| HELLO keeps working through PSS with `fake_ndm = 1` | Relay mode is feasible from inside the game: the next step is emulating `nwm::UDS` over the relay. |

### Results (2026-10-15, one console, Luma3DS v13.4)

- **Sockets work from inside Pokémon X**, once the socket buffer comes from the game's heap region: `socInit` refused the plugin's own memory (`0xE0A01BF5`, invalid address) and accepted 128 KB allocated with `svcControlMemory` at `0x0ffa0000`. HELLO round trips on the LAN took 30–150 ms (370–610 ms for the first after Wi-Fi comes up).
- **`fake_ndm = 0`:** when PSS called EnterExclusiveState, real NDM went to local mode, AC dropped (status 3 → 1) and every HELLO failed for 51 s, until LeaveExclusiveState brought Wi-Fi back.
- **`fake_ndm = 1`:** real NDM stayed in normal mode and HELLO kept working through the game's EnterExclusiveState. Real NWM then refused InitializeWithVersion (`0xE06113F9`) because the console was not in local mode, and the game showed "communication failed". Relay mode avoids this by answering `nwm::UDS` in the shim instead of forwarding it.
- **Known issue, not the shim:** exiting the game sometimes crashes in `_thread_begin` on core 1 (crash dumps 6 and 8). It is CTRPF's task-scheduler worker for core 1, which never runs while Pokémon X gives core 1 no time, and starts only after the game exit has unmapped the plugin. It happens after the game has closed.

## 8. Relay mode (Phase 4)

In relay mode the shim answers the game's `nwm::UDS` calls itself, with `shim/core`'s emulator (`src/uds.c`, a C port of `internal/udsfake`'s fake console), and carries hosting, scans, joins and data over the relay. Real NWM never sees a UDS call, and NDM stays in normal mode, so Wi-Fi stays up.

How it is put together:

- One emulator serves both of the game's `nwm::UDS` sessions. Each session thread passes its requests to `shim_uds_handle`. A relay thread, started with the plugin, brings up sockets (section 7's heap-region buffer), feeds the relay socket to the emulator, and runs its retries and keepalives every 20 ms.
- Calls that need the relay (hosting, scans, joins, leaving, app data) block their session thread until the relay answers, as NWM blocks the game while the radio works. Everything else is answered at once from local state.
- The emulator creates the status-change event (returned by InitializeWithVersion) and one event per Bind, and signals them on every status change and every frame received.
- Scans return 802.11 beacons in Azahar's layout, one per network the relay lists. The node list that NWM would encrypt goes into the beacon in plain text, and the shim's DecryptBeaconData reads it back.
- If the relay does not answer HELLO within 3 s of InitializeWithVersion, the emulator fails it with `0xE06113F9`, what real NWM returned in section 7, so the game shows its usual communication error.
- Results follow `battle-hw-01`: 0x0021 succeeds, 0x0023 returns a word ending `1fff`, DestroyNetwork with no network and PullPacket after leaving fail with `0xC8A113EA`. Commands it does not know succeed with no output and are logged as `not emulated`.

On a PC, `TestReplayTracesShim` (in `internal/udsfake`) runs each of the seven two-console traces, including `relay-hw-01` from this mode on hardware, through two copies of the emulator (`shim/core/tests/uds_replay.c`) against the Go relay, as the game's side: it sends hardware-shaped IPC requests, parses the beacons and joins through DecryptBeaconData's node list. It checks every reply against the recording, every frame byte for byte, the reply layouts, and that every status change was signalled.

### On two consoles

1. Start the relay on a PC on the same LAN (section 7, step 1).
2. On both consoles write `sdmc:/udsrelay/shim.ini`:

   ```ini
   mode = relay
   relay = <PC's LAN address>:28300
   lobby = <the key>
   ```

3. Install the latest `udsrelay-shim.3gx` on both, Wi-Fi on, launch the game. A notification says **relay connected** once HELLO is answered; the relay's terminal logs both sessions.
4. Open PSS on both and start a link trade from one. The relay logs the hosted networks; the other console should find the trader in its list.
5. Trade, then battle. The plugin menu's Status shows the relay RTT, the UDS status and node bitmask, and frame counts.
6. Copy both logs off and redact them as in section 4 (scans now contain only the other console's beacon). `not emulated` lines name commands the emulator does not handle yet.

What to look for: whether the client sees the host's network in PSS, whether the trade completes, and the reason code a client gets when the host quits mid-match (the emulator reports 4, unconfirmed).

### Results (2026-10-15, two consoles on one LAN, Pokémon X, Luma3DS v13.4)

- **0.3.0:** PSS found the other console and a full trade completed over the relay, but both consoles then froze on a black screen (music still playing). After DisconnectNetwork or DestroyNetwork and the Unbinds, the game waits for its receive thread, which waits on the bind event. Real NWM signals the bind events on teardown and Unbind, so the thread pulls once, gets `0xC8A113EA` and exits (`battle-hw-01` shows that pull). The emulator did not signal them.
- **0.3.1** signals every bind's event on Unbind, Shutdown, DestroyNetwork, DisconnectNetwork and a lost network. A full trade and a full battle completed over the relay, each returning to PSS normally, with no warnings and no unhandled commands in either log. The host sent 884 frames and the client received 883; the client sent 773 and the host received all 773. The replay test now checks these signals. The redacted logs are `recon/traces/relay-hw-01/`.
- **0.3.2** came from replaying `relay-hw-01`. Received frames were capped at 32 across all binds, where NWM limits each bind to the buffer size the game asks for (0x2E30 bytes in X/Y, roughly 180 of its frames). The replay delivers frames in bursts, as internet latency will, and lost frames past the cap. The queue now holds 64 KB of 120-byte chunks with NWM's per-bind byte limit. Not yet run on hardware.
- Still unconfirmed: the reason code a client sees when the host quits mid-match, and play across two networks (Phase 5).

## Troubleshooting

| Log or symptom | Meaning | Next step |
| --- | --- | --- |
| `no srv:GetServiceHandle function found` | The finder did not recognise this game's SDK code. | Dump the game's code region (for example over Rosalina's GDB stub) so the finder's pattern can be widened. |
| `GetServiceHandle candidate ... hook result N` with N ≠ 0 | CTRPF refused the hook. | The `HookResult` value in `Hook.hpp` says why. |
| `could not tell which argument is the output handle` | The game's GetServiceHandle signature differs from what the shim expects. | The logged r0–r3 values show the arguments; adjust `shim_pick_out_arg`. |
| No "session(s) intercepted" notification | The game opened `nwm::UDS` some other way, or not yet. | Check the log for hook lines; open PSS or start a local trade. |
| `forwarding 0x.... failed` | The real service rejected the forwarded request. | Usually a static buffer larger than the shim's 0x800-byte slots; raise `kSlotSize` in `shim.cpp`. |
| Game freezes when local play starts | A forwarding thread is stuck or crashed. | Disable the plugin loader to confirm, then send the log. |
| `InitializeWithVersion: the relay does not answer` (relay mode) | No HELLO answer within 3 s: Wi-Fi off, wrong address or lobby key, or the relay not running. | Look for `relay did not answer HELLO` lines ("did not verify" means a wrong lobby key) and the relay's terminal; section 7's table applies. |
| `nwm::UDS command 0x.... not emulated` (relay mode) | The game used a command the emulator does not model; it got success and no data. | Note what the game did next and add the command to `shim/core/src/uds.c`. |

## Status and risks

- **Tested on two consoles** (Pokémon X, Luma3DS v13.4): a full trade and a full battle in passthrough, with no dropped lines or failed forwards. Exiting the game after a trip to the HOME menu corrupted the plugin heap even with the shim disabled, so `main()` no longer deletes the menu at exit. The Makefile works around two mismatches between libctrpf 0.8.0 and current libctru: it adds libctru's `include/3ds` for libctrpf's `"types.h"` includes, and defines the `NORETURN` macro libctru dropped. `svcControlService` comes from a minimal `Sources/luma_svc.hpp`, since devkitPro does not install Luma's `csvc.h`.
- **Same-process sessions:** the game and the shim share a process, so the game's client session and the shim's server session live in one handle table. The kernel allows this, but it is the first thing to suspect if requests never arrive.
- **One hook point:** if a game obtains `nwm::UDS` before the plugin loads, or through a path other than GetServiceHandle, the shim never sees it. The plugin loader runs before the game's entry point, so the first case should not happen.
