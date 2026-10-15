# UDS command trace: trade-01

Captured 2026-10-15 14:03:21 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 127.7s | 878 | kujira-1 | host |
| client | 123.7s | 886 | kujira-1 | client |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +9.073s | +9.124s | +10.277s | — | +45.392s | +45.367s | +10.276s |
| client | +9.079s | +9.124s | +9.512s | +45.285s | +45.355s | +45.433s | +9.511s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (21 times, first at +9.124s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (22 times, first at +9.124s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +41.442s: 184 bytes `010a000118020241776a30483692dba29c0401003905c7989d6163324b006100…`
- **host: SetApplicationData** at +41.446s: 184 bytes `010a000118020241776a30483692dba29c0401003905c7989d6163324b006100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 2 | 2 | +40.871s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 22 [1] | 24 [2] | +9.511s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 1 | +117.081s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 6 | 4 | +41.364s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 56 | 59 | +9.512s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 2 | 0 | +41.442s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 2 | +41.410s | yes |
| nwm::UDS | `0x0013` | Unbind | 4 | 4 | +117.082s | yes |
| nwm::UDS | `0x0014` | PullPacket | 439 [1] | 463 [2] | +45.317s | yes |
| nwm::UDS | `0x0017` | SendTo | 244 | 224 | +45.355s | yes |
| nwm::UDS | `0x001A` | GetChannel | 1 | 0 | +41.411s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 3 | +9.073s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 22 | 22 | +9.124s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 1 | +45.285s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 42 | 44 | +9.812s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 22 [22] | 22 [22] | +9.124s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 2 [2] | 2 [2] | +121.971s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 3 | +9.072s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 2 | +40.872s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +0.552s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +9.124s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +11.182s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +12.551s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +14.608s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.662s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.347s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.577s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.633s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.376s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +27.116s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +28.489s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +30.544s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +31.229s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.283s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +35.340s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +38.082s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +40.821s host: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +41.411s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +118.803s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +121.198s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +121.971s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +121.972s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +122.874s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +124.244s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +124.927s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +127.670s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +9.124s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +9.972s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +12.019s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +13.389s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +14.758s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +16.130s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.499s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +18.871s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +20.241s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +22.982s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +25.038s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.410s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +27.906s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +29.961s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +30.647s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +33.429s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +34.116s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +36.857s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +38.914s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +41.654s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +44.396s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +45.299s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +117.081s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +117.081s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +120.438s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +121.976s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +121.976s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +123.479s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar

## Events signalled

| Event | host | client |
| --- | --- | --- |
| HLE Pause Event: uds::ConnectToNetwork | 0 | 1 |
| NWM::BindNodeEvent2 | 225 | 245 |
| NWM::BindNodeEvent4 | 1 | 1 |
| NWM::connection_status_event | 45 | 46 |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 1 | — | 243 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 224 | 46 / 69 / 1349 |
| host | recv | bind 2 | 2 | — | 224 | 46 / 69 / 1349 |
| host | send | 243 | 2 | 0x02 | 244 | 46 / 69 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +0.552s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +0.552s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +9.072s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +40.872s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +41.364s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +119.268s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +121.151s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +0.820s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.820s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +1.388s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +1.388s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +9.077s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +44.447s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +44.923s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +117.567s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +120.388s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
