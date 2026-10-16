# Phase 6 runbook: the udsrelay sysmodule

Goal (design doc, Phase 6 exit): three games play local wireless over the relay with no per-game code. The plugin (`shim/plugin`) finds and hooks one function in each game's code; the sysmodule takes over its job for every game, from outside the game.

## How it works

Three files on the SD card, in `sdmc:/luma/sysmodules/`, switched on by one Luma setting:

| File | What |
| --- | --- |
| `0004013000F00D02.cxi` | The udsrelay sysmodule (`sysmodule/source/main.c` plus `shim/core`). It registers `uds:rly` and `ndm:rly`. `uds:rly` is the same `nwm::UDS` emulator the plugin runs in relay mode. `ndm:rly` forwards `ndm:u` to the real NDM, except the exclusive-state calls, which it fakes so Wi-Fi stays up. A relay thread starts sockets the first time a game opens `uds:rly`. |
| `sm.cxi` | Luma3DS v13.4's service manager with `luma/sm-redirect.patch`. An application (title ID `00040000…`) asking for `nwm::UDS` or `ndm:u` gets `uds:rly` or `ndm:rly` instead, while those are registered. System processes, including the sysmodule, keep the real services. |
| `pm.cxi` | Luma3DS v13.4's process manager with `luma/pm-autolaunch.patch`. After starting NS at boot, it starts title `0004013000F00D02` once and ignores failure. |

With "Enable loading external FIRMs and modules" on, Luma replaces its built-in `sm` and `pm` with the files above, and its loader runs the sysmodule from its `.cxi` (nothing is installed). With the option off, or the files gone, the console boots exactly as before.

Without `relay` and `lobby` in `sdmc:/udsrelay/shim.ini`, the sysmodule logs that and exits without registering anything, so games keep real local wireless. The sysmodule always runs in relay mode; it ignores `mode`, `ports` and `fake_ndm`.

Logs go to `sdmc:/udsrelay/logs/sysmodule-<UTC time>.log`, one per boot, in the same `UDSTRACE` format as the plugin's, so `udstrace` reads them.

One game runs at a time, so one emulator serves whichever game is running; each InitializeWithVersion starts it over, and leaves any network a crashed game left behind.

## 1. Toolchain

As for the plugin (`shim/RUNBOOK.md` section 1), plus `makerom`, which packages `.cxi` files. Build it from Project_CTR:

```bash
git clone --depth 1 https://github.com/3DSGuy/Project_CTR.git ~/src/Project_CTR
```

```bash
make -C ~/src/Project_CTR/makerom deps && make -C ~/src/Project_CTR/makerom
```

```bash
sudo cp ~/src/Project_CTR/makerom/bin/makerom /opt/devkitpro/tools/bin/
```

`build.sh` also finds it in `~/src/Project_CTR/makerom/bin` if it is not installed.

## 2. Build

```bash
sysmodule/build.sh
```

It stages `shim/core` and `sysmodule` in `/tmp/udsrelay-sysmodule-build`, clones Luma3DS v13.4 there (once) and applies the two patches, then writes the three files to `sysmodule/out/`.

### SD card package

`package.sh` builds and then zips everything a console needs, in the SD card's layout: the three `.cxi` files under `luma/sysmodules/`, and a `udsrelay/shim.ini` for the chosen mode. For relay mode, it reads the lobby key from `--lobby-file`, else `$UDSRELAY_LOBBY`, else asks for it without echoing, so the key stays out of shell history:

```bash
sysmodule/package.sh relay --relay relay.example.net:28300
```

For trace mode (below), no relay or key:

```bash
sysmodule/package.sh trace
```

The zip goes to `sysmodule/out/udsrelay-sd-<mode>.zip` (git-ignored). A relay-mode zip contains the lobby key, so share it only with people in the lobby. `--payload-max N` sets how much of each frame is logged (default 64), `--no-build` packages what is already in `sysmodule/out`, `-o` picks another path, and `--force` overwrites an existing zip.

Unzip it, then copy `luma/` and `udsrelay/` onto the SD card root, **merging** with the folders already there. Windows Explorer and FTP clients merge folders. In macOS Finder, hold Option while dropping and choose Merge: Replace would delete the existing `luma` folder, including Luma's own settings. An existing `shim.ini` is overwritten.

## 3. Install and test, one step at a time

A broken `sm` or `pm` stops the console from booting. To recover, hold Select while powering on to open Luma's configuration and turn "Enable loading external FIRMs and modules" off, or delete the files from the SD card on a PC. Do steps a and b on one console first.

Turn the plugin off for these tests (Rosalina → Plugin Loader → Disabled), so only the sysmodule handles UDS.

a. **Patched sm and pm alone.** Copy `sm.cxi` and `pm.cxi` to `sdmc:/luma/sysmodules/`, without the sysmodule. In Luma's configuration (hold Select at power-on), turn on "Enable loading external FIRMs and modules". The console should boot normally, and Pokémon X's local wireless should work as usual between two nearby consoles: nothing is registered under `uds:rly`, so `sm` redirects nothing. This tests that a missing sysmodule is harmless.

b. **The sysmodule.** Copy `0004013000F00D02.cxi` to `sdmc:/luma/sysmodules/` too, keep `shim.ini` with `relay` and `lobby`, and reboot. Before starting a game, `sdmc:/udsrelay/logs/` should have a new `sysmodule-….log` that says `serving uds:rly and ndm:rly`.

c. **Pokémon X/Y over the relay**, as in Phase 5: start the relay, open PSS on both consoles, trade, then battle. The sysmodule's log should show `serving a nwm::UDS session` when PSS starts, then `relay mode: …`, the hosted networks and joins.

d. **Other games:** Mario Kart 7 (one race, then a second race), and Smash Bros. for 3DS (one match). Copy the logs off after each game, named after it.

Send both consoles' logs from each step that fails, and from c and d either way.

## Trace mode: recording Download Play's local wireless

Some games run local play through the Download Play system module (title `0004013000002802`) rather than calling `nwm::UDS` themselves. Mario Kart 7 is one: in Azahar (`recon/traces/mk7-azahar-01`) it never sends a `nwm::UDS` request, and on a console in relay mode it asks for local mode, then backs out 30 ms later with "turn on wireless". The patched `sm` gives Download Play its own names, `uds:dlp` and `ndm:dlp`, which relay mode does not register, so it keeps the real services and fails, as real NWM refuses local play while NDM is faked.

Trace mode records what Download Play actually does, before the emulator is taught to serve it. With `mode = trace` in `sdmc:/udsrelay/shim.ini`, the sysmodule registers all four names and answers nothing itself. It forwards every request from the game and from Download Play to the real NWM and NDM and logs it, as the Phase 2 plugin did for Pokémon. Local wireless works as usual, so the two consoles must be side by side. `relay` and `lobby` are not needed.

1. On both consoles: the latest `sm.cxi`, `pm.cxi` and `0004013000F00D02.cxi` in `sdmc:/luma/sysmodules/`, plugin loader off, and `shim.ini` with:

   ```ini
   mode = trace
   payload_max = 64
   ```

2. Reboot both. Each `sysmodule-….log` should say `in trace mode`.
3. Mario Kart 7 on both, Multiplayer → Local Play: one creates a group, the other joins, then race once.
4. Copy both logs. `Dispatch` lines say which sessions serve `the game` and which `Download Play`, with their `sess=` numbers.
5. Set `mode = relay` again afterwards.

### What the Mario Kart 7 trace showed (`recon/traces/mk7-trace-hw-01`)

Two consoles side by side, Local Play, one race, sysmodule 0.5.0 in trace mode; local wireless worked normally through it.

- **Lobby: Download Play.** The host's Download Play hosts its own network (comm ID `0x00002810`, ID8 `0x55`, 8 nodes, no app data, 18-byte passphrase) and broadcasts on channel 1. The joining Download Play connects first as a **spectator** (conn_type 2) to listen on channel 1, then disconnects and rejoins as a client (conn_type 1) on channel 2. The host then calls UpdateNetworkAttribute (bitmask `0x0002`, flag 1), unbinds and destroys the network. Download Play never uses `ndm:u`.
- **Race: the game itself.** After Download Play shuts down, Mario Kart opens its own two `nwm::UDS` sessions, hosts comm ID `0x00030810` (ID8 0, 8 nodes) and sends on channel 243 with flags `0x03`, mostly broadcast, while polling GetNodeInformation over a thousand times.
- **The two never overlap**, so one emulator serves them in turn.
- **No deprecated commands**: everything is a command the emulator handles. One new result: PullPacket on a bind that no longer exists while in a network returns `0xE10113EA` (the emulator now does too; `0xC8A113EA` stays for not being in a network).
- Download Play leaves stack leftovers, including pieces of the console's friend-code seed, in the unused upper bits of SendTo's and Bind's narrow arguments; `udstrace redact` now masks those bits in every trace.

### Relay mode and Download Play (0.5.1)

Relay mode also registers `uds:dlp`, so Download Play's `nwm::UDS` goes to the emulator like the game's, and games that run their lobby through Download Play can use the relay. `ndm:dlp` stays unregistered (Download Play keeps the real NDM, which it never called in the trace). The start-up line reads `serving uds:rly, ndm:rly and uds:dlp`. Not yet tested on hardware: the spectator join is the first use of that path.

## What to look for

| Symptom | Meaning |
| --- | --- |
| No `sysmodule-….log` at all | The sysmodule did not start: `pm.cxi` or the `.cxi` is not where Luma looks, the option is off, or it crashed at start (Luma shows a crash screen; send the dump from `sdmc:/luma/dumps/arm11/`). |
| Log says `not serving` | `shim.ini` lacks `relay` or `lobby`, or the key is bad. |
| Log never shows `serving a nwm::UDS session` once the game runs | The redirect did not happen: the game reached the real `nwm::UDS` (check that `sm.cxi` is in place), or the game uses another route to local wireless. |
| `no free session slot` | The game holds more than four sessions to the two services at once; raise `MAX_SESSIONS` in `main.c`. |
| `socInit failed` or `cannot allocate a socket buffer` | The SYSTEM-region allocation and the fallback both failed; the log has the result codes. |
| `nwm::UDS command 0x.... not emulated` | The game uses a command the emulator does not handle yet; it got success and no data. Expected for some games; note what the game did next. |

## Results so far (2026-10-15)

- Pokémon X/Y traded between a sysmodule console and a plugin console, and Pokémon X/Y and Smash Bros. for 3DS played over the internet. Donkey Kong Country Returns 3D played but slowly. Animal Crossing: New Leaf – Welcome amiibo (comm ID `0x00198D10`) connected and received data steadily, then the game itself disconnected after about 41 s, twice; the internet round trip limiting its transfer is the leading suspect, untested.
- Fixed on the way: `-nocodepadding` (copied from Luma's own modules) packed the code file, so Luma's loader mapped every constant and global to the wrong address and startup stalled before `main` (found from a Rosalina memory dump). The module now waits for SD access and a valid system clock instead of crashing or naming its log with a wrapped-around date, and reopens its log after write errors.
- Seen but not explained: the log stopped growing once ftpd started (0.4.1). 0.4.2 notes the failing write's result code and reopens the file.

## Not done yet

- Sleep mode: the sysmodule does not handle sleep notifications; after waking, the relay resets its session and it says HELLO again, but a game in local play at that moment will see the network drop.
- No status display: the plugin's menu and notifications have no counterpart; the log is the only output.
- Mario Kart 7 over the relay (0.5.1): built, not yet tested on hardware.
- Memory: about 950 KB of BASE-region memory (mostly eight sessions' receive buffers), plus a 128 KB socket buffer from the SYSTEM region. 0.5.0 started on both consoles.
