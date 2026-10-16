# UDS command trace: internet-hw-01 (two consoles on different networks, X and Y, shim relay mode)

Captured 2026-10-15 20:13:17 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 510.3s | 4145 | 0004000000055D00 | P1 |
| client | 396.7s | 3563 | 0004000000055E00 | P2 |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +11.655s | +11.700s | +13.516s | — | +210.557s | +210.552s | +13.474s |
| client | +145.005s | +145.049s | +145.653s | +210.371s | +210.516s | +210.622s | +145.525s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (209 times, first at +11.700s).
- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x00000000`, app data 0 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (2 times, first at +208.452s).
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (96 times, first at +145.049s).
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0x06299dec`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **client: joined (conn_type 1) via ConnectToNetwork**: wlan comm ID `0x00055d10`, ID8 `0x01`, max nodes 4, attributes `0x8000`, network ID `0xce347efa`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300`.
- **host: SetApplicationData** at +208.510s: 184 bytes `25140001180202412abc5d5664ac40e29c0401003905c78e2cf1633250003100…`
- **host: SetApplicationData** at +208.551s: 184 bytes `25140001180202412abc5d5664ac40e29c0401003905c78e2cf1633250003100…`
- **host: SetApplicationData** at +344.359s: 184 bytes `ef720001180202412abc5d5664ac40e29c0401003905c798b4f1633250003100…`
- **host: SetApplicationData** at +344.400s: 184 bytes `ef720001180202412abc5d5664ac40e29c0401003905c798b4f1633250003100…`

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0003` | Shutdown | 6 | 5 | +207.698s | yes |
| nwm::UDS | `0x0008` | DestroyNetwork | 215 [4] | 101 [5] | +13.474s | yes |
| nwm::UDS | `0x000A` | DisconnectNetwork | 0 | 2 | +313.805s | yes |
| nwm::UDS | `0x000B` | GetConnectionStatus | 10 | 6 | +208.390s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 524 | 226 | +13.516s | yes |
| nwm::UDS | `0x0010` | SetApplicationData | 4 | 0 | +208.510s | yes |
| nwm::UDS | `0x0012` | Bind | 4 | 4 | +208.450s | yes |
| nwm::UDS | `0x0013` | Unbind | 8 | 8 | +313.882s | yes |
| nwm::UDS | `0x0014` | PullPacket | 1795 [2] | 1904 [3] | +210.505s | yes |
| nwm::UDS | `0x0017` | SendTo | 955 | 921 | +210.516s | yes |
| nwm::UDS | `0x001A` | GetChannel | 2 | 0 | +208.485s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 6 | 5 | +11.655s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 211 | 96 | +11.700s | yes |
| nwm::UDS | `0x001E` | ConnectToNetwork | 0 | 2 | +210.371s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 164 | 165 | +145.136s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 211 | 96 | +11.699s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 14 | 8 | +13.514s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 6 | 5 | +11.648s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 6 | 5 | +207.700s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +2.123s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +207.663s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +316.210s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +343.555s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +475.849s host: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +494.490s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +510.256s host: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +209.489s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +210.474s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +313.881s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +345.537s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +346.560s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +474.163s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +474.164s client: PullPacket(bind=2 max=0x5dc) -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)
- +521.869s client: DestroyNetwork() -> 0xC8A113EA (desc=1002 module=68 summary=5 level=25)

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
| client | recv | bind 2 | 1 | — | 954 | 46 / 69 / 1349 |
| client | send | 243 | 1 | 0x02 | 921 | 46 / 69 / 1349 |
| host | recv | bind 2 | 2 | — | 919 | 46 / 69 / 1349 |
| host | send | 243 | 2 | 0x02 | 955 | 46 / 69 / 1349 |

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +2.123s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +2.124s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +11.648s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +207.700s host: ndm:u LeaveExclusiveState(words=0x20,0x35) -> OK
- +208.385s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +316.716s host: ndm:u LeaveExclusiveState(words=0x20,0x35) -> OK
- +318.646s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +343.599s host: ndm:u LeaveExclusiveState(words=0x20,0x35) -> OK
- +344.251s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +476.332s host: ndm:u LeaveExclusiveState(words=0x20,0x35) -> OK
- +478.860s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +494.539s host: ndm:u LeaveExclusiveState(words=0x20,0x35) -> OK
- +499.841s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +510.300s host: ndm:u LeaveExclusiveState(words=0x20,0x35) -> OK
- +125.204s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +125.205s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +127.771s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +127.772s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +144.997s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +209.528s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +210.181s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +314.382s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +319.287s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +345.578s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +346.230s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +474.650s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
- +477.081s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +521.907s client: ndm:u LeaveExclusiveState(words=0x20,0x28) -> OK
