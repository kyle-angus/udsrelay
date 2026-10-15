# UDS command trace: xy-01

Captured 2026-10-15 14:27:34 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 138.2s | 901 | kujira-1 | host |
| client | 134.2s | 899 | kujira-2 | client |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +10.849s | +10.896s | +12.435s | — | +46.303s | +46.282s | +12.435s |
| client | +10.869s | +10.916s | +11.351s | +46.197s | +46.270s | +46.347s | +11.350s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (24 times, first at +10.896s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (23 times, first at +10.916s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +44.157s: 184 bytes `8766000118020241776a30483692dba29c0401003905c7984a6763324b006100…`
- **host: SetApplicationData** at +44.162s: 184 bytes `8766000118020241776a30483692dba29c0401003905c7984a6763324b006100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 2 | 2 | +43.624s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 26 [1] | 24 [2] | +11.350s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 1 | +127.622s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 6 | 4 | +44.080s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 56 | 55 | +11.351s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 2 | 0 | +44.157s | yes |
| nwm::UDS | `0x0012` | Bind | 2 | 2 | +44.128s | yes |
| nwm::UDS | `0x0013` | Unbind | 4 | 4 | +127.623s | yes |
| nwm::UDS | `0x0014` | PullPacket | 443 [1] | 483 [1] | +46.233s | yes |
| nwm::UDS | `0x0017` | SendTo | 253 | 227 | +46.270s | yes |
| nwm::UDS | `0x001A` | GetChannel | 1 | 0 | +44.129s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 3 | 3 | +10.849s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 25 | 23 | +10.896s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 1 | +46.197s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 42 | 36 | +11.653s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 25 [25] | 23 [23] | +10.896s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 2 [2] | 2 [2] | +131.048s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 3 | +10.848s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 2 | +43.624s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +0.568s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +10.896s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +13.639s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +14.324s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.378s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +17.065s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.119s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +20.491s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.175s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +23.917s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +26.659s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +28.713s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +30.087s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +32.142s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +34.882s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +36.938s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +37.622s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +39.679s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +42.420s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +43.573s host: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +44.128s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +129.744s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +131.712s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +132.398s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +132.399s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +133.000s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +135.055s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +135.739s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +136.425s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +137.797s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +10.916s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +13.811s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +14.470s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.531s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +18.584s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.273s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +22.011s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.068s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +25.436s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.807s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +28.866s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +30.921s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +32.978s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.661s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +35.718s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +38.457s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +39.827s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +41.885s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +42.573s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +45.312s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +46.215s client: DestroyNetwork() -> 0xC8A111EA (desc=490 module=68 summary=5 level=25)
- +127.623s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +130.663s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +131.048s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +131.049s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +131.774s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +133.125s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +135.178s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar

## Events signalled

| Event | host | client |
| --- | --- | --- |
| HLE Pause Event: uds::ConnectToNetwork | 0 | 1 |
| NWM::BindNodeEvent2 | 228 | 254 |
| NWM::BindNodeEvent4 | 1 | 1 |
| NWM::connection_status_event | 52 | 47 |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 1 | — | 253 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 227 | 46 / 69 / 1349 |
| host | recv | bind 2 | 2 | — | 227 | 46 / 69 / 1349 |
| host | send | 243 | 2 | 0x02 | 253 | 46 / 69 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +0.568s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +0.568s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +10.848s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +43.624s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +44.079s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +130.244s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +131.662s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +0.994s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.994s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +1.562s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +1.562s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +10.868s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +45.362s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +45.832s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +128.110s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +130.616s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
