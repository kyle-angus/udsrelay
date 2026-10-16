# UDS command trace: mk7-trace-hw-01 (two consoles side by side, Mario Kart 7 local play, sysmodule trace mode: Download Play lobby, then the game's race)

Captured 2026-10-16 13:18:26 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 146.6s | 4423 | sysmodule | P1 |
| client | 133.4s | 10392 | sysmodule | P1 |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +10.163s | +21.689s | +10.708s | — | +22.859s | +25.735s | +20.850s |
| client | +12.207s | — | +12.726s | +23.029s | +25.712s | +23.556s | +23.761s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00002810`, ID8 `0x55`, max nodes 8, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 18 bytes `306b6d407473612475686d79316130736100`.
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00030810`, ID8 `0x00`, max nodes 8, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 9 bytes `304545384638000000`.
- **client: joined (conn_type 2) via ConnectToNetwork**: wlan comm ID `0x00002810`, ID8 `0x55`, max nodes 8, attributes `0x8000`, network ID `0x81a0769b`, app data 0 bytes, passphrase 18 bytes `306b6d407473612475686d79316130736100`.
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00002810`, ID8 `0x55`, max nodes 8, attributes `0x8000`, network ID `0x81a0769b`, app data 0 bytes, passphrase 18 bytes `306b6d407473612475686d79316130736100`.
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00030810`, ID8 `0x00`, max nodes 8, attributes `0x8000`, network ID `0x821d7913`, app data 0 bytes, passphrase 9 bytes `304545384638000000`.

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 2 | 1 | +20.850s | yes |
| nwm::UDS | `0x0007` | UpdateNetworkAttribute | 1 | 0 | +29.546s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 1 | 0 | +29.952s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 2 | +23.761s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 1190 | 2306 | +22.798s | yes |
| nwm::UDS | `0x000D` | GetNodeInformation | 1181 | 2284 | +25.688s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 24 | 32 | +10.708s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 3 | +22.801s | yes |
| nwm::UDS | `0x0013` | Unbind | 2 | 3 | +23.759s | yes |
| nwm::UDS | `0x0014` | PullPacket | 1023 [1] | 3851 | +22.810s | yes |
| nwm::UDS | `0x0017` | SendTo | 980 | 1894 | +22.859s | yes |
| nwm::UDS | `0x001A` | GetChannel | 1 | 0 | +31.436s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 2 | +10.163s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 2 | 0 | +21.689s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 3 | +23.029s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 0 | 4 | +23.021s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 2 | 0 | +21.686s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 4 | 3 | +9.872s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 1 | +20.968s | n/a |
| ndm:u | `0x0004` | LockState | 1 | 1 | +135.572s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.002s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021).

## Failures and unimplemented commands

- +29.932s host: PullPacket(bind=0 max=0x5bc) -> 0xE10113EA (desc=1002 module=68 summary=8 level=28)

## Events signalled

| Event | host | client |
| --- | --- | --- |

## Data channels

- host Bind: bind=2 recv_buf=0x3c00 chan=2 node=broadcast -> OK +event handle
- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=1 recv_buf=0x3c00 chan=1 node=1 -> OK +event handle
- client Bind: bind=3 recv_buf=0x3c00 chan=2 node=1 -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 1 | 1 | — | 5 | 768 / 1448 / 1464 |
| client | recv | bind 2 | 1 | — | 1705 | 30 / 607 / 1219 |
| client | recv | bind 3 | 1 | — | 8 | 20 / 24 / 32 |
| client | send | 2 | 1 | 0x01 | 7 | 24 / 24 / 28 |
| client | send | 243 | 1 | 0x03 | 53 | 30 / 61 / 216 |
| client | send | 243 | broadcast | 0x03 | 1834 | 535 / 607 / 1219 |
| host | recv | bind 2 | 2 | — | 547 | 24 / 607 / 1219 |
| host | send | 1 | broadcast | 0x01 | 137 | 768 / 1448 / 1464 |
| host | send | 2 | 2 | 0x01 | 8 | 20 / 24 / 32 |
| host | send | 243 | 2 | 0x03 | 16 | 31 / 73 / 73 |
| host | send | 243 | broadcast | 0x03 | 819 | 535 / 607 / 1219 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.002s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +9.872s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +20.968s host: ndm:u LeaveExclusiveState(words=0x20,0x27) -> OK
- +21.020s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +30.118s host: ndm:u LeaveExclusiveState(words=0x20,0x27) -> OK
- +30.186s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +146.589s host: ndm:u EnterExclusiveState(state=Infrastructure) -> OK
- +146.590s host: ndm:u LockState(words=0x20,0x29) -> OK
- +2.181s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +2.182s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +11.664s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +29.984s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +30.377s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +135.572s client: ndm:u EnterExclusiveState(state=Infrastructure) -> OK
- +135.572s client: ndm:u LockState(words=0x20,0x2a) -> OK
