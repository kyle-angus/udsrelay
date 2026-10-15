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

   It does not build on macOS as-is, so apply two small patches first. The bundled dynalo library only recognises Linux and Windows, but its Linux backend is plain `dlopen`, so let macOS use it:

   ```bash
   sed -i '' 's/defined(_LINUX)$/defined(_LINUX) || defined(__APPLE__)/' ~/src/3gxtool/extern/dynalo/include/dynalo/detail/config.hpp
   ```

   The project links with `-static`, which macOS does not support, so limit that to other platforms:

   ```bash
   sed -i '' 's/^set(CMAKE_EXE_LINKER_FLAGS "-static-libgcc -static-libstdc++ -static")$/if(NOT APPLE)\
   set(CMAKE_EXE_LINKER_FLAGS "-static-libgcc -static-libstdc++ -static")\
   endif()/' ~/src/3gxtool/CMakeLists.txt
   ```

   ```bash
   cmake -S ~/src/3gxtool -B ~/src/3gxtool/build -DCMAKE_POLICY_VERSION_MINIMUM=3.5 && cmake --build ~/src/3gxtool/build
   ```

   The policy flag is needed because the bundled yaml-cpp declares a CMake minimum below 3.5, which CMake 4 rejects.

   Put the resulting `3gxtool` binary on your `PATH`, for example in `/opt/devkitpro/tools/bin`.

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
5. Copy `sdmc:/udsrelay/logs/*.log` off each console (SD card reader, or ftpd), into `recon/traces/<session>/console-host.log` and `console-client.log`.

## 5. Compare against the Azahar trace

```bash
go run ./cmd/udstrace compare azahar=recon/traces/trade-01/host.log console=recon/traces/trade-hw-01/console-host.log
```

```bash
go run ./cmd/udstrace compare azahar=recon/traces/trade-01/client.log console=recon/traces/trade-hw-01/console-client.log
```

`compare` reports commands present in only one trace, commands whose result codes differ, changes in the order of significant calls (polls and data packets are left out of the order check), and network parameter differences. It exits 1 on any difference.

For a merged two-console timeline, line the clocks up with `-shift`. Console clocks differ by seconds, so pick an offset (in milliseconds; the value below is only an example) that puts the client's ConnectToNetwork just before the host's next GetConnectionStatus shows two nodes:

```bash
go run ./cmd/udstrace timeline -shift client=-2350 host=recon/traces/trade-hw-01/console-host.log client=recon/traces/trade-hw-01/console-client.log
```

Expected differences from Azahar, which are not shim bugs:

- Azahar replies to `ConnectToNetwork` asynchronously and logs `wake` lines. On hardware the reply comes back directly.
- Azahar stubs `EjectClient` and `EjectSpectators` and has no handler for some commands. Real NWM returns real results.
- Console logs have no `signal` lines: the shim cannot see NWM signal its events without stealing those signals from the game.

## 6. Done when

- A full trade and a full battle complete between two consoles with the shim active, with no dropped lines.
- `compare` shows no commands, results or call-order differences against the Azahar trace beyond the expected ones above, and each remaining difference is written up in the design doc.

## Troubleshooting

| Log or symptom | Meaning | Next step |
| --- | --- | --- |
| `no srv:GetServiceHandle function found` | The finder did not recognise this game's SDK code. | Dump the game's code region (for example over Rosalina's GDB stub) so the finder's pattern can be widened. |
| `GetServiceHandle candidate ... hook result N` with N ≠ 0 | CTRPF refused the hook. | The `HookResult` value in `Hook.hpp` says why. |
| `could not tell which argument is the output handle` | The game's GetServiceHandle signature differs from what the shim expects. | The logged r0–r3 values show the arguments; adjust `shim_pick_out_arg`. |
| No "session(s) intercepted" notification | The game opened `nwm::UDS` some other way, or not yet. | Check the log for hook lines; open PSS or start a local trade. |
| `forwarding 0x.... failed` | The real service rejected the forwarded request. | Usually a static buffer larger than the shim's 0x800-byte slots; raise `kSlotSize` in `shim.cpp`. |
| Game freezes when local play starts | A forwarding thread is stuck or crashed. | Disable the plugin loader to confirm, then send the log. |

## Status and risks

- **Not run on hardware yet.** The plugin builds with devkitARM (GCC 16.1) and libctrpf 0.8.0, but nothing has run on a console. The Makefile works around two mismatches between libctrpf 0.8.0 and current libctru: it adds libctru's `include/3ds` for libctrpf's `"types.h"` includes, and defines the `NORETURN` macro libctru dropped. `svcControlService` comes from a minimal `Sources/luma_svc.hpp`, since devkitPro does not install Luma's `csvc.h`.
- **Same-process sessions:** the game and the shim share a process, so the game's client session and the shim's server session live in one handle table. The kernel allows this, but it is the first thing to suspect if requests never arrive.
- **One hook point:** if a game obtains `nwm::UDS` before the plugin loads, or through a path other than GetServiceHandle, the shim never sees it. The plugin loader runs before the game's entry point, so the first case should not happen.
