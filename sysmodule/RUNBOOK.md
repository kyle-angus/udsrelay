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

## 3. Install and test, one step at a time

A broken `sm` or `pm` stops the console from booting. To recover, hold Select while powering on to open Luma's configuration and turn "Enable loading external FIRMs and modules" off, or delete the files from the SD card on a PC. Do steps a and b on one console first.

Turn the plugin off for these tests (Rosalina → Plugin Loader → Disabled), so only the sysmodule handles UDS.

a. **Patched sm and pm alone.** Copy `sm.cxi` and `pm.cxi` to `sdmc:/luma/sysmodules/`, without the sysmodule. In Luma's configuration (hold Select at power-on), turn on "Enable loading external FIRMs and modules". The console should boot normally, and Pokémon X's local wireless should work as usual between two nearby consoles: nothing is registered under `uds:rly`, so `sm` redirects nothing. This tests that a missing sysmodule is harmless.

b. **The sysmodule.** Copy `0004013000F00D02.cxi` to `sdmc:/luma/sysmodules/` too, keep `shim.ini` with `relay` and `lobby`, and reboot. Before starting a game, `sdmc:/udsrelay/logs/` should have a new `sysmodule-….log` that says `serving uds:rly and ndm:rly`.

c. **Pokémon X/Y over the relay**, as in Phase 5: start the relay, open PSS on both consoles, trade, then battle. The sysmodule's log should show `serving a nwm::UDS session` when PSS starts, then `relay mode: …`, the hosted networks and joins.

d. **Other games:** Mario Kart 7 (one race, then a second race), and Smash Bros. for 3DS (one match). Copy the logs off after each game, named after it.

Send both consoles' logs from each step that fails, and from c and d either way.

## What to look for

| Symptom | Meaning |
| --- | --- |
| No `sysmodule-….log` at all | The sysmodule did not start: `pm.cxi` or the `.cxi` is not where Luma looks, the option is off, or it crashed at start (Luma shows a crash screen; send the dump from `sdmc:/luma/dumps/arm11/`). |
| Log says `not serving` | `shim.ini` lacks `relay` or `lobby`, or the key is bad. |
| Log never shows `serving a nwm::UDS session` once the game runs | The redirect did not happen: the game reached the real `nwm::UDS` (check that `sm.cxi` is in place), or the game uses another route to local wireless. |
| `no free session slot` | The game holds more than four sessions to the two services at once; raise `MAX_SESSIONS` in `main.c`. |
| `socInit failed` or `cannot allocate a socket buffer` | The SYSTEM-region allocation and the fallback both failed; the log has the result codes. |
| `nwm::UDS command 0x.... not emulated` | The game uses a command the emulator does not handle yet; it got success and no data. Expected for some games; note what the game did next. |

## Not done yet

- Not run on hardware.
- Sleep mode: the sysmodule does not handle sleep notifications; after waking, the relay resets its session and it says HELLO again, but a game in local play at that moment will see the network drop.
- No status display: the plugin's menu and notifications have no counterpart; the log is the only output.
- Memory: about 670 KB of BASE-region memory (mostly four sessions' receive buffers), plus a 128 KB socket buffer from the SYSTEM region. Fine on both models in principle; untested.
