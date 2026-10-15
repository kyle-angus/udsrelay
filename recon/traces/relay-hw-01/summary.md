# UDS command trace: relay-hw-01 (two consoles, shim relay mode)

Captured 2026-10-15 18:38:44 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 324.9s | 2901 | 0004000000055D00 | P1 |
| client | 328.4s | 3084 | 0004000000055D00 | P2 |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +23.284s | +23.332s | +25.141s | — | +60.148s | +60.141s | +25.104s |
| client | +23.326s | +23.370s | +24.001s | +60.045s | +60.123s | +60.208s | +23.951s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (61 times, first at +23.332s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (2 times, first at +56.087s).
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (65 times, first at +23.370s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0xf1469177`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x2b7bf50c`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +56.145s: 184 bytes `9fd30001180202412abc5d5664ac40e29c0401003905c79872e8633250003100…`
- **host: SetApplicationData** at +56.186s: 184 bytes `9fd30001180202412abc5d5664ac40e29c0401003905c79872e8633250003100…`
- **host: SetApplicationData** at +172.940s: 184 bytes `033d0001180202412abc5d5664ac40e29c0401003905c78ee7e8633250003100…`
- **host: SetApplicationData** at +172.965s: 184 bytes `033d0001180202412abc5d5664ac40e29c0401003905c78ee7e8633250003100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 5 | 5 | +55.383s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 66 [3] | 70 [5] | +23.951s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 2 | +132.291s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 10 | 6 | +56.025s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 148 | 167 | +24.001s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 4 | 0 | +56.145s | yes |
| nwm::UDS | `0x0012` | Bind | 4 | 4 | +56.086s | yes |
| nwm::UDS | `0x0013` | Unbind | 8 | 8 | +132.318s | yes |
| nwm::UDS | `0x0014` | PullPacket | 1495 [2] | 1757 [3] | +60.141s | yes |
| nwm::UDS | `0x0017` | SendTo | 884 | 773 | +60.123s | yes |
| nwm::UDS | `0x001A` | GetChannel | 2 | 0 | +56.123s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 5 | 5 | +23.284s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 63 | 65 | +23.332s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 2 | +60.045s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 124 | 135 | +24.054s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 63 | 65 | +23.329s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 6 | 6 | +24.000s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 5 | 5 | +23.278s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 5 | 5 | +55.385s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.007s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +3.072s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +55.338s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +134.061s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +172.144s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +320.011s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +328.204s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +59.212s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +60.079s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +132.317s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +174.580s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +175.481s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +318.131s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +318.133s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +328.341s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)

## Events signalled

| Event | host | client |
| --- | --- | --- |

## Data channels

- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- host Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=2 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle
- client Bind: bind=4 recv_buf=0x2e30 chan=243 node=broadcast -> OK +event handle

| Instance | Direction | Channel / bind | Peer node | Flags | Packets | Size min / median / max (bytes) |
| --- | --- | --- | --- | --- | --- | --- |
| client | recv | bind 2 | 1 | — | 883 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 773 | 46 / 69 / 1349 |
| host | recv | bind 2 | 2 | — | 773 | 46 / 69 / 1349 |
| host | send | 243 | 2 | 0x02 | 884 | 46 / 69 / 1349 |

## NDM and AC calls

- +3.390s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +3.390s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +5.478s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +5.479s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +23.278s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +55.385s host: ndm:u LeaveExclusiveState(words=0x20,0x29) -> OK
- +56.020s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +134.583s host: ndm:u LeaveExclusiveState(words=0x20,0x29) -> OK
- +141.459s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +172.180s host: ndm:u LeaveExclusiveState(words=0x20,0x29) -> OK
- +172.815s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +320.506s host: ndm:u LeaveExclusiveState(words=0x20,0x29) -> OK
- +321.953s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +328.252s host: ndm:u LeaveExclusiveState(words=0x20,0x29) -> OK
- +0.000s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.007s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +3.072s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +3.073s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +23.318s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +59.253s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +59.922s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +132.785s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +140.782s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +174.627s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +175.280s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +318.625s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +321.171s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +328.393s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
