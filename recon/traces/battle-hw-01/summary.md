# UDS command trace: battle-hw-01 (two consoles, shim passthrough)

Captured 2026-10-15 11:09:58 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 208.8s | 1600 | 0004000000055D00 | P1 |
| client | 207.1s | 1666 | 0004000000055D00 | P2 |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +14.925s | +15.399s | +17.000s | — | +58.868s | +58.855s | +16.983s |
| client | +15.258s | +15.742s | +17.623s | +58.768s | +59.062s | +59.148s | +17.604s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (20 times, first at +15.399s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (19 times, first at +15.742s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x681e4898`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +55.418s: 184 bytes `f0760001180202412abc5d5664ac40e29c0401003905c78e437f633250003100…`
- **host: SetApplicationData** at +55.442s: 184 bytes `f0760001180202412abc5d5664ac40e29c0401003905c78e437f633250003100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 3 | 3 | +53.419s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 23 [2] | 22 [3] | +16.983s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 1 | +197.650s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 5 | 4 | +54.267s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 40 | 48 | +17.000s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 2 | 0 | +55.418s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 2 | +54.339s | yes |
| nwm::UDS | `0x0013` | Unbind | 4 | 4 | +197.676s | yes |
| nwm::UDS | `0x0014` | PullPacket | 892 [1] | 1033 [3] | +58.813s | yes |
| nwm::UDS | `0x0017` | SendTo | 549 | 471 | +58.868s | yes |
| nwm::UDS | `0x001A` | GetChannel | 1 | 0 | +55.396s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 3 | +14.925s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 21 | 19 | +15.399s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 1 | +58.768s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 20 | 22 | +19.695s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 21 | 19 | +15.398s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 4 | 4 | +16.997s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 3 | +14.695s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 3 | 3 | +53.522s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.001s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +2.139s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +53.377s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +199.865s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +208.703s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +57.346s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +59.025s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +197.672s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +197.674s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +197.675s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +208.808s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)

## Events signalled

| Event | host | client |
| --- | --- | --- |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 1 | — | 530 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 471 | 46 / 69 / 1349 |
| host | recv | bind 2 | 2 | — | 452 | 46 / 69 / 1349 |
| host | send | 243 | 2 | 0x02 | 549 | 46 / 69 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.001s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +2.139s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +2.140s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +14.695s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +53.522s host: ndm:u LeaveExclusiveState(words=0x20,0x37) -> OK
- +54.156s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +200.461s host: ndm:u LeaveExclusiveState(words=0x20,0x37) -> OK
- +203.173s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +208.842s host: ndm:u LeaveExclusiveState(words=0x20,0x37) -> OK
- +1.896s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +1.897s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +4.672s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +4.673s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +14.971s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +57.492s client: ndm:u LeaveExclusiveState(words=0x20,0x49) -> OK
- +58.176s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +198.280s client: ndm:u LeaveExclusiveState(words=0x20,0x49) -> OK
- +202.378s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +208.951s client: ndm:u LeaveExclusiveState(words=0x20,0x49) -> OK
