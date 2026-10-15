# UDS command trace: drop-01

Captured 2026-10-15 14:13:37 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 122.4s | 663 | kujira-1 | host |
| client | 88.3s | 585 | kujira-1 | client |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +9.529s | +9.577s | +10.414s | +42.055s | +42.127s | +42.205s | +10.414s |
| client | +9.536s | +9.585s | +11.123s | — | +42.167s | +42.146s | +11.122s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (22 times, first at +9.577s).
- **host: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (15 times, first at +9.585s).
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: SetApplicationData** at +39.640s: 184 bytes `7fe200011802024183c837e6a174c2c19c0401003905c798fe6363324b006100…`
- **client: SetApplicationData** at +39.644s: 184 bytes `7fe200011802024183c837e6a174c2c19c0401003905c798fe6363324b006100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 2 | 1 | +39.101s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 23 [2] | 16 [1] | +10.414s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 1 | 0 | +117.555s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 4 | 4 | +39.559s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 53 | 43 | +10.414s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 0 | 2 | +39.640s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 1 | +39.606s | yes |
| nwm::UDS | `0x0013` | Unbind | 4 | 0 | +117.556s | yes |
| nwm::UDS | `0x0014` | PullPacket | 307 [2] | 279 | +42.112s | yes |
| nwm::UDS | `0x0017` | SendTo | 173 | 163 | +42.127s | yes |
| nwm::UDS | `0x001A` | GetChannel | 0 | 1 | +39.607s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 2 | +9.529s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 22 | 16 | +9.577s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 1 | 0 | +42.055s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 35 | 34 | +10.715s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 22 [22] | 16 [16] | +9.577s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 2 [2] | 0 | +119.013s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 2 | +9.528s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 1 | +39.101s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +0.569s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +9.577s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +11.016s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +11.698s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +13.757s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +15.829s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.198s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +19.939s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.997s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +22.680s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.053s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +26.107s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.791s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +29.533s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +32.276s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.647s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +34.332s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +37.071s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +39.814s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +41.186s host: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +42.072s host: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +117.555s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +117.556s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +118.290s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +119.013s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +119.014s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +119.608s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +120.978s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +122.351s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +9.585s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +12.328s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +15.069s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +15.751s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.125s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.808s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +20.551s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.922s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +23.978s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.719s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +28.087s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +29.458s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +31.514s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +34.255s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +36.314s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +39.055s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +39.607s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar

## Events signalled

| Event | host | client |
| --- | --- | --- |
| HLE Pause Event: uds::ConnectToNetwork | 1 | 0 |
| NWM::BindNodeEvent2 | 164 | 142 |
| NWM::BindNodeEvent4 | 1 | 0 |
| NWM::connection_status_event | 45 | 32 |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 2 | — | 142 | 46 / 69 / 1349 |
| client | send | 243 | 2 | 0x02 | 163 | 46 / 69 / 1349 |
| host | recv | bind 2 | 1 | — | 163 | 46 / 69 / 1349 |
| host | send | 243 | 1 | 0x02 | 173 | 46 / 69 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +0.569s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +0.569s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +9.528s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +41.234s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +41.689s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +118.039s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +118.119s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +0.998s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.998s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +1.545s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +1.545s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +9.534s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +39.101s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +39.559s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
