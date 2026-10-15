# UDS command trace: drop-02

Captured 2026-10-15 14:17:51 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 86.0s | 525 | kujira-1 | host |
| client | 56.5s | 455 | kujira-1 | client |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +9.978s | +10.026s | +10.865s | — | +38.447s | +38.427s | +10.864s |
| client | +9.979s | +10.026s | +11.564s | +38.308s | +38.399s | +38.476s | +11.564s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (18 times, first at +10.026s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (16 times, first at +10.026s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +34.024s: 184 bytes `543a000118020241776a30483692dba29c0401003905c798f56463324b006100…`
- **host: SetApplicationData** at +34.029s: 184 bytes `543a000118020241776a30483692dba29c0401003905c798f56463324b006100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 2 | 1 | +33.460s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 20 [1] | 18 [2] | +10.864s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 5 | 3 | +33.933s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 42 | 41 | +10.865s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 2 | 0 | +34.024s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 1 | +33.982s | yes |
| nwm::UDS | `0x0013` | Unbind | 4 | 0 | +80.448s | yes |
| nwm::UDS | `0x0014` | PullPacket | 195 [1] | 216 | +38.391s | yes |
| nwm::UDS | `0x0017` | SendTo | 168 | 100 | +38.399s | yes |
| nwm::UDS | `0x001A` | GetChannel | 1 | 0 | +33.982s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 2 | +9.978s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 19 | 16 | +10.026s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 1 | +38.308s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 32 | 33 | +11.167s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 19 [19] | 16 [16] | +10.026s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 2 [2] | 0 | +82.371s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 2 | +9.976s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 1 | +33.461s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +0.569s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +10.026s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +11.464s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +14.207s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.275s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.965s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +18.333s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.018s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.760s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +23.814s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.503s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +25.871s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +27.245s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +29.298s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +29.986s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +32.038s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.410s host: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +33.982s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +80.447s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +81.215s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +82.371s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +82.371s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +83.573s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +84.258s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +10.026s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +12.769s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +15.507s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.880s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.566s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.621s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +20.307s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +20.990s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +23.047s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +23.732s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +25.105s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +27.160s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +29.900s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +32.641s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.327s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +34.697s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +37.441s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +38.346s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)

## Events signalled

| Event | host | client |
| --- | --- | --- |
| HLE Pause Event: uds::ConnectToNetwork | 0 | 1 |
| NWM::BindNodeEvent2 | 101 | 121 |
| NWM::BindNodeEvent4 | 1 | 0 |
| NWM::connection_status_event | 39 | 33 |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 1 | — | 121 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 100 | 46 / 69 / 1349 |
| host | recv | bind 2 | 2 | — | 100 | 46 / 69 / 1349 |
| host | send | 243 | 2 | 0x02 | 168 | 46 / 82 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +0.569s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +0.569s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +9.976s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +33.461s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +33.933s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +80.949s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +81.031s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +0.987s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.988s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +1.538s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +1.538s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +9.978s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +37.489s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +37.947s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
