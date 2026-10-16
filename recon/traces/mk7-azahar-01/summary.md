# UDS command trace: mk7-azahar-01

Captured 2026-10-16 14:56:38 UTC from 2 instance logs with recon/azahar/udstrace.patch.

## Sessions

| Instance | Duration | Calls | Processes calling UDS | Username |
| --- | --- | --- | --- | --- |
| host | 103.2s | 7 | MarioKar |  |
| client | 112.5s | 7 | MarioKar |  |

## Milestones

| Instance | Init | Host | Scan | Join | Data sent | Data received | Leave |
| --- | --- | --- | --- | --- | --- | --- | --- |
| host | — | — | — | — | — | — | — |
| client | — | — | — | — | — | — | — |

## Network parameters

No BeginHostingNetwork or ConnectToNetwork payloads were logged.

## Commands used

Counts per instance; failures (non-zero result or unimplemented) in brackets. "Planned" means the design doc's UDS emulation table covers it.

| Port | ID | Command | host | client | First seen | Planned |
| --- | --- | --- | --- | --- | --- | --- |
| ndm:u | `0x0001` | EnterExclusiveState | 3 | 3 | +14.613s | n/a |
| ndm:u | `0x0002` | LeaveExclusiveState | 2 | 2 | +37.650s | n/a |
| ndm:u | `0x0006` | SuspendDaemons | 1 | 1 | +0.000s | n/a |
| ndm:u | `0x0014` | OverrideDefaultDaemons | 1 | 1 | +0.000s | n/a |

## Failures and unimplemented commands

None.

## Events signalled

| Event | host | client |
| --- | --- | --- |
| NWM::BindNodeEvent1 | 0 | 192 |
| NWM::connection_status_event | 65 | 64 |
| dlp_connect_to_beacon | 0 | 32 |

## Data channels


No data packets were sent or received.

## NDM and AC calls

- +0.000s host: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +0.000s host: ndm:u SuspendDaemons(words=0x6) -> OK
- +14.613s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +37.650s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +93.096s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +103.111s host: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +103.164s host: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +1.165s client: ndm:u OverrideDefaultDaemons(words=0xf) -> OK
- +1.165s client: ndm:u SuspendDaemons(words=0x6) -> OK
- +16.060s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +38.503s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +95.759s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
- +111.133s client: ndm:u LeaveExclusiveState(words=0x20,0xb) -> OK
- +113.690s client: ndm:u EnterExclusiveState(state=LocalCommunications) -> OK
