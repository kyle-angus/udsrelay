# UDS command trace: idle-01

Captured 2026-10-15 13:57:43 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 94.0s | 360 | kujira-1 | host |
| client | 90.2s | 345 | kujira-1 | client |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | +11.197s | +11.244s | +13.979s | — | — | — | +13.978s |
| client | +11.191s | +11.237s | +11.623s | — | — | — | +11.622s |

## Network parameters

- **host: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (46 times, first at +11.244s).
- **client: hosted via BeginHostingNetwork**: wlan comm ID `0x00055d10`, ID8 `0x02`, max nodes 2, attributes `0x8000`, network ID `0x00000000`, app data 184 bytes, passphrase 13 bytes `474e6742774d63566f74597300` (42 times, first at +11.237s).

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| nwm::UDS | `0x0008` | DestroyNetwork | 46 | 41 | +11.622s | yes |
| nwm::UDS | `0x000F` | RecvBeaconBroadcastData | 118 | 114 | +11.623s | yes |
| nwm::UDS | `0x001B` | InitializeWithVersion | 1 | 1 | +11.191s | yes |
| nwm::UDS | `0x001D` | BeginHostingNetwork | 46 | 42 | +11.237s | yes |
| nwm::UDS | `0x001F` | DecryptBeaconData | 96 | 98 | +11.925s | yes |
| nwm::UDS | `0x0021` | SetProbeResponseParam | 46 [46] | 42 [42] | +11.237s | **no** |
| nwm::UDS | `0x0023` | cmd_0023 | 2 [2] | 2 [2] | +55.648s | **no** |
| ndm:u | `0x0001` | EnterExclusiveState | 1 | 1 | +11.189s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0007` | ResumeDaemons | 2 | 2 | +0.658s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

Not in the design table yet: SetProbeResponseParam (0x0021), cmd_0023 (0x0023).

## Failures and unimplemented commands

- +11.244s host: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +14.988s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +15.676s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +17.730s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +20.471s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +23.213s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.584s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +25.955s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.641s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +28.743s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +30.114s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +30.800s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +33.540s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +36.280s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +37.652s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +39.115s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +41.170s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +42.543s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +44.597s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +45.969s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +48.025s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +48.709s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +50.078s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +50.764s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +53.507s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +54.877s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +55.648s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +55.649s host: cmd_0023() -> UNIMPLEMENTED in Azahar
- +56.547s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +57.919s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +59.974s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +62.032s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +64.085s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +66.826s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +69.569s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +72.309s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +72.995s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +75.737s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +78.479s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +80.534s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +83.275s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +86.016s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +87.385s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +88.071s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +88.756s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +90.127s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +90.815s host: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +92.884s host: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +11.237s client: SetProbeResponseParam(words=0x321f00,0x0) -> UNIMPLEMENTED in Azahar
- +12.789s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +16.502s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +19.244s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +21.987s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +22.673s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +24.041s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +25.414s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +26.098s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +28.156s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +30.946s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +33.686s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +35.740s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +38.481s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +39.852s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +40.540s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +42.592s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +43.965s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +44.649s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +47.392s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +49.447s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +52.189s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +53.557s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +55.614s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +58.355s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +59.040s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +60.196s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +60.197s client: cmd_0023() -> UNIMPLEMENTED in Azahar
- +61.400s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +62.768s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +64.140s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +65.512s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +67.565s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +70.306s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +73.047s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +74.419s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +77.158s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +79.217s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +81.957s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +83.327s client: SetProbeResponseParam(words=0x321f00,0x40000) -> UNIMPLEMENTED in Azahar
- +85.384s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +88.125s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +88.812s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar
- +90.865s client: SetProbeResponseParam(words=0x321f00,0x20000) -> UNIMPLEMENTED in Azahar

## Events signalled

| Event | host | client |
| --- | --- | --- |
| NWM::connection_status_event | 92 | 83 |

## Data channels


No data packets were sent or received.

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +0.658s host: ndm:u ResumeDaemons(words=0x2) -> OK
- +0.658s host: ndm:u ResumeDaemons(words=0x4) -> OK
- +11.195s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +0.628s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.628s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +1.265s client: ndm:u ResumeDaemons(words=0x2) -> OK
- +1.265s client: ndm:u ResumeDaemons(words=0x4) -> OK
- +11.189s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
