# UDS command trace: battle-01

Captured 2026-10-15 14:07:33 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 225.1s | 1784 | kujira-1 | host |
| client | 220.5s | 1810 | kujira-1 | client |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +16.601s | +16.648s | +17.491s | — | +61.231s | +61.210s | +17.491s |
| client | +16.619s | +16.667s | +17.528s | +61.096s | +61.184s | +61.261s | +17.521s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (30 times, first at +16.648s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (29 times, first at +16.667s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +57.533s: 184 bytes `df13000118020241776a30483692dba29c0401003905c78ea16263324b006100…`
- **host: SetApplicationData** at +57.538s: 184 bytes `df13000118020241776a30483692dba29c0401003905c78ea16263324b006100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 2 | 2 | +56.981s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 32 [1] | 31 [2] | +17.491s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 1 | +214.193s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 6 | 4 | +57.454s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 66 | 72 | +17.491s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 2 | 0 | +57.533s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 2 | +57.500s | yes |
| nwm::UDS | `0x0013` | Unbind | 4 | 4 | +214.194s | yes |
| nwm::UDS | `0x0014` | PullPacket | 998 [1] | 1060 [2] | +61.210s | yes |
| nwm::UDS | `0x0017` | SendTo | 544 | 504 | +61.184s | yes |
| nwm::UDS | `0x001A` | GetChannel | 1 | 0 | +57.500s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 3 | +16.601s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 31 | 29 | +16.648s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 1 | +61.096s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 51 | 55 | +17.787s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 31 [31] | 29 [29] | +16.648s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 2 [2] | 4 [4] | +58.721s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 3 | +16.600s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 2 | +56.981s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +0.584s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +16.648s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +18.086s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +18.770s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.510s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +22.198s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.254s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.940s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.993s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +27.680s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +30.421s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +31.794s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.849s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +36.590s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +37.959s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +40.014s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +42.760s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +43.443s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +44.811s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +46.181s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +47.554s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +48.241s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +50.296s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +51.664s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +52.350s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +55.090s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +55.777s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +56.930s host: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +57.500s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +216.569s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +218.550s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +219.238s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +219.238s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +219.839s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +221.207s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +223.264s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +16.667s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +18.121s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +18.805s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.491s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.548s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +23.603s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.345s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +28.401s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +31.142s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +32.514s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.883s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +36.624s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +37.993s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +40.736s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +41.422s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +42.790s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +44.163s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +44.848s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +45.533s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +46.902s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +48.275s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +49.646s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +50.327s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +53.073s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +55.126s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +57.183s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +58.721s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +58.722s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +60.224s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +61.131s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +214.193s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +214.194s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +217.703s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +218.471s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +218.471s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +219.376s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +220.747s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar

## Events signalled

| Event | host | client |
| --- | --- | --- |
| HLE Pause Event: uds::ConnectToNetwork | 0 | 1 |
| NWM::BindNodeEvent2 | 505 | 544 |
| NWM::BindNodeEvent4 | 1 | 1 |
| NWM::connection_status_event | 64 | 60 |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 1 | — | 543 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 504 | 46 / 63 / 1349 |
| host | recv | bind 2 | 2 | — | 504 | 46 / 63 / 1349 |
| host | send | 243 | 2 | 0x02 | 544 | 46 / 69 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +0.584s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +0.584s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +16.600s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +56.981s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +57.453s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +217.031s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +218.499s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +1.298s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +1.299s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +1.867s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +1.867s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +16.617s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +60.274s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +60.731s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +214.680s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +217.655s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
