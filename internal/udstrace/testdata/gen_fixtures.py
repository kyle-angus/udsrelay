#!/usr/bin/env python3
"""Generate synthetic Azahar logs in the exact format udstrace.patch emits.

These are NOT real captures. They model a two-instance local trade so the parser and reports
can be tested before a real Phase 1 trace exists. Field values (comm ID, passphrase, payloads)
are invented. Run from anywhere: python3 -I gen_fixtures.py
"""
import os
import struct

HERE = os.path.dirname(os.path.abspath(__file__))
T0 = 1_792_000_000_000_000  # wall clock, microseconds since epoch
COMM_ID = 0x0012_3456
PASSPHRASE = b"FIXTURE-PASSPHRASE-NOT-REAL"


class Log:
    def __init__(self, name, offset_us):
        self.name = name
        self.lines = []
        self.wall = T0 + offset_us
        self.emu = 1.0  # emulator-relative seconds in the stock prefix

    def tick(self, us):
        self.wall += us
        self.emu += us / 1e6

    def _line(self, cls, level, src, msg):
        secs = int(self.emu)
        frac = int(round((self.emu - secs) * 1e6))
        self.lines.append(f"[{secs:4d}.{frac:06d}] {cls} <{level}> {src}: {msg}")

    def trace(self, src, body):
        self._line("Service", "Info", src, f"UDSTRACE v=1 {body}")

    def note(self, cls, level, src, msg):
        self._line(cls, level, src, msg)

    def call(self, port, cmd, name, req_hdr, req_words, rsp_hdr, rsp_words,
             deferred=False, between=None):
        words = ",".join(f"{w:#010x}" for w in [req_hdr] + req_words)
        self.trace("core/hle/service/service.cpp:HandleSyncRequest:192",
                   f'kind=req wall={self.wall} port={port} cmd={cmd:#06x} name="{name}" tid=33 '
                   f'proc="pokemonxy" hdr={req_hdr:#010x} words={words}')
        self.tick(40)
        if between:
            between()
        rwords = ",".join(f"{w:#010x}" for w in [rsp_hdr] + rsp_words)
        self.trace("core/hle/service/service.cpp:HandleSyncRequest:219",
                   f"kind=rsp wall={self.wall} port={port} cmd={cmd:#06x} hdr={rsp_hdr:#010x} "
                   f"deferred={1 if deferred else 0} words={rwords}")
        self.tick(200)

    def unimplemented(self, port, cmd, name, req_hdr, req_words):
        """A command Azahar has no handler for: req, Azahar's own error line, then unimpl."""
        words = ",".join(f"{w:#010x}" for w in [req_hdr] + req_words)
        self.trace("core/hle/service/service.cpp:HandleSyncRequest:192",
                   f'kind=req wall={self.wall} port={port} cmd={cmd:#06x} name="{name}" tid=33 '
                   f'proc="pokemonxy" hdr={req_hdr:#010x} words={words}')
        self.note("Service", "Error", "core/hle/service/service.cpp:ReportUnimplementedFunction:170",
                  f"unknown / unimplemented function '{name}': port='{port}' cmd_buf={{[0]={req_hdr:#x}}}")
        self.trace("core/hle/service/service.cpp:HandleSyncRequest:200",
                   f"kind=unimpl wall={self.wall} port={port} cmd={cmd:#06x} hdr={req_hdr:#010x}")

    def blob(self, cmd, tag, data, extra=""):
        sep = " " if extra else ""
        self.trace("core/hle/service/nwm/nwm_uds.cpp:TraceBlob:44",
                   f"kind=blob wall={self.wall} port=nwm::UDS cmd={cmd:#06x} tag={tag}{sep}{extra} "
                   f"size={len(data)} hex={data.hex()}")

    def signal(self, event):
        self.trace("core/hle/service/nwm/nwm_uds.cpp:SignalEventAsync:1690",
                   f'kind=signal wall={self.wall} port=nwm::UDS event="{event}"')

    def wake(self, cmd, hdr, words):
        w = ",".join(f"{x:#010x}" for x in [hdr] + words)
        self.trace("core/hle/service/nwm/nwm_uds.cpp:TraceWake:1385",
                   f"kind=wake wall={self.wall} port=nwm::UDS cmd={cmd:#06x} hdr={hdr:#010x} words={w}")

    def write(self):
        with open(os.path.join(HERE, self.name), "w") as f:
            f.write("\n".join(self.lines) + "\n")


def hdr(cmd, normal, translate):
    return (cmd << 16) | (normal << 6) | translate


def node_info_words(username):
    raw = struct.pack("<Q", 0x1122334455667788)
    raw += username.encode("utf-16-le").ljust(20, b"\0")
    raw += b"\0" * 4 + struct.pack("<H", 0) + b"\0" * 6
    assert len(raw) == 40
    return list(struct.unpack("<10I", raw))


def network_info(max_nodes, appdata):
    b = bytearray(0x108)
    b[0:6] = bytes([0x40, 0xF4, 0x07, 0xAA, 0xBB, 0x01])
    b[6] = 11
    b[0xC:0xF] = bytes([0x00, 0x1F, 0x32])
    b[0xF] = 0x15
    struct.pack_into(">I", b, 0x10, COMM_ID)
    b[0x14] = 0x02
    struct.pack_into(">H", b, 0x16, 0x0000)
    struct.pack_into(">I", b, 0x18, 0x0BADF00D)
    b[0x1C] = 1
    b[0x1D] = max_nodes
    b[0x3F] = len(appdata)
    b[0x40:0x40 + len(appdata)] = appdata
    return bytes(b)


def conn_status(status, reason, node, changed, nodes, total, maxn, bitmask):
    ns = (nodes + [0] * 16)[:16]
    raw = struct.pack("<IIHH", status, reason, node, changed)
    raw += struct.pack("<16H", *ns)
    raw += struct.pack("<BBH", total, maxn, bitmask)
    assert len(raw) == 0x30
    return list(struct.unpack("<12I", raw))


APPDATA = bytes(range(0x10, 0x30))
UDS = "nwm::UDS"
NDM = "ndm:u"


def common_init(log, username):
    log.call(NDM, 0x1, "EnterExclusiveState", hdr(0x1, 1, 2), [2, 0x20, 0], hdr(0x1, 1, 0), [0])
    log.note("Service.NDM", "Warning", "core/hle/service/ndm/ndm_u.cpp:EnterExclusiveState:21",
             "(STUBBED) exclusive_state=0x00000002")
    log.call(UDS, 0x1B, "InitializeWithVersion", hdr(0x1B, 12, 2),
             [0x3000] + node_info_words(username) + [0x400, 0, 0x55],
             hdr(0x1B, 1, 2), [0, 0, 0x60])


def host_log():
    log = Log("host.log", 0)
    common_init(log, "Serena")
    log.call(UDS, 0x10, "SetApplicationData", hdr(0x10, 1, 2), [len(APPDATA), 0, 0], hdr(0x10, 1, 0), [0],
             between=lambda: log.blob(0x10, "appdata", APPDATA))
    ni = network_info(2, APPDATA)
    log.call(UDS, 0x1D, "BeginHostingNetwork", hdr(0x1D, 1, 4), [len(PASSPHRASE), 0, 0, 0, 0],
             hdr(0x1D, 1, 0), [0],
             between=lambda: (log.blob(0x1D, "netinfo", ni), log.blob(0x1D, "passphrase", PASSPHRASE),
                              log.signal("NWM::connection_status_event")))
    log.call(UDS, 0x12, "Bind", hdr(0x12, 4, 0), [1, 0x2000, 1, 0xFFFF], hdr(0x12, 1, 2), [0, 0, 0x61])
    waiting = conn_status(6, 1, 1, 1, [1], 1, 2, 0x0001)
    for _ in range(3):
        log.call(UDS, 0xB, "GetConnectionStatus", hdr(0xB, 0, 0), [], hdr(0xB, 13, 0), [0] + waiting)
        log.tick(100_000)
    log.tick(400_000)  # client joins at ~0.75 s
    log.signal("NWM::connection_status_event")
    joined = conn_status(6, 1, 1, 2, [1, 2], 2, 2, 0x0003)
    log.call(UDS, 0xB, "GetConnectionStatus", hdr(0xB, 0, 0), [], hdr(0xB, 13, 0), [0] + joined)
    log.unimplemented(UDS, 0x15, "SetMaxSendDelay", hdr(0x15, 1, 0), [10])
    log.tick(5_000)
    for i in range(4):
        payload = bytes([0xA0, i]) + b"trade-offer" + bytes(range(i * 3))
        log.call(UDS, 0x17, "SendTo", hdr(0x17, 6, 2), [0, 2, 1, 0, len(payload), 0x00, 0, 0],
                 hdr(0x17, 1, 0), [0],
                 between=lambda p=payload: log.blob(0x17, "send", p, "dest=0x0002 chan=1 flags=0x00"))
        log.call(UDS, 0x14, "PullPacket", hdr(0x14, 3, 0), [1, 0x5F4, 0x5F4], hdr(0x14, 3, 2), [0, 0, 0, 0, 0])
        log.tick(30_000)
        reply = bytes([0xB0, i]) + b"trade-ack"
        log.signal("NWM::BindNodeEvent1")
        log.call(UDS, 0x14, "PullPacket", hdr(0x14, 3, 0), [1, 0x5F4, 0x5F4],
                 hdr(0x14, 3, 2), [0, len(reply), 2, 0, 0],
                 between=lambda r=reply: log.blob(0x14, "recv", r, "bind=1 src=0x0002"))
        log.tick(30_000)
    log.tick(200_000)
    log.call(UDS, 0x8, "DestroyNetwork", hdr(0x8, 0, 0), [], hdr(0x8, 1, 0), [0],
             between=lambda: log.signal("NWM::connection_status_event"))
    log.call(UDS, 0x3, "Shutdown", hdr(0x3, 0, 0), [], hdr(0x3, 1, 0), [0])
    log.call(NDM, 0x2, "LeaveExclusiveState", hdr(0x2, 0, 2), [0x20, 0], hdr(0x2, 1, 0), [0])
    return log


def client_log():
    log = Log("client.log", 150_000)
    common_init(log, "Calem")
    scan_req = [0x10000, 0, 0, 0xFFFFFFFF, 0xFFFF] + [0] * 9 + [COMM_ID, 0x02, 0, 0x24, 0, 0x10000, 0x7000]
    for _ in range(2):
        log.call(UDS, 0xF, "RecvBeaconBroadcastData", hdr(0xF, 16, 4), scan_req,
                 hdr(0xF, 1, 2), [0, 0x10004, 0x7000], deferred=True)
        log.note("Service.NWM", "Debug", "core/hle/service/nwm/nwm_uds.cpp:RecvBeaconBroadcastData:689",
                 f"called out_buffer_size=0x00010000, wlan_comm_id=0x{COMM_ID:08X}, id=0x00000002, unk1=0x00000000, unk2=0x00000000, offset=0")
        log.tick(300_000)
    ni = network_info(2, APPDATA)
    req_hdr = hdr(0x1E, 2, 4)
    log.call(UDS, 0x1E, "ConnectToNetwork", req_hdr, [1, len(PASSPHRASE), 0, 0, 0, 0],
             req_hdr, [1, len(PASSPHRASE), 0, 0, 0, 0], deferred=True,
             between=lambda: (log.blob(0x1E, "netinfo", ni, "conn_type=1"),
                              log.blob(0x1E, "passphrase", PASSPHRASE)))
    log.tick(20_000)
    log.signal("NWM::connection_status_event")
    log.wake(0x1E, hdr(0x1E, 1, 0), [0])
    log.note("Service.NWM", "Debug", "core/hle/service/nwm/nwm_uds.cpp:WakeUp:1376", "connection sequence finished")
    log.tick(1_000)
    log.call(UDS, 0x12, "Bind", hdr(0x12, 4, 0), [1, 0x2000, 1, 0xFFFF], hdr(0x12, 1, 2), [0, 0, 0x61])
    joined = conn_status(9, 1, 2, 3, [1, 2], 2, 2, 0x0003)
    log.call(UDS, 0xB, "GetConnectionStatus", hdr(0xB, 0, 0), [], hdr(0xB, 13, 0), [0] + joined)
    for i in range(4):
        log.tick(20_000)
        offer = bytes([0xA0, i]) + b"trade-offer" + bytes(range(i * 3))
        log.signal("NWM::BindNodeEvent1")
        log.call(UDS, 0x14, "PullPacket", hdr(0x14, 3, 0), [1, 0x5F4, 0x5F4],
                 hdr(0x14, 3, 2), [0, len(offer), 1, 0, 0],
                 between=lambda p=offer: log.blob(0x14, "recv", p, "bind=1 src=0x0001"))
        reply = bytes([0xB0, i]) + b"trade-ack"
        log.call(UDS, 0x17, "SendTo", hdr(0x17, 6, 2), [0, 1, 1, 0, len(reply), 0x00, 0, 0],
                 hdr(0x17, 1, 0), [0],
                 between=lambda r=reply: log.blob(0x17, "send", r, "dest=0x0001 chan=1 flags=0x00"))
        log.tick(40_000)
    log.tick(150_000)
    lost = conn_status(3, 4, 0, 0, [], 0, 0, 0)
    log.call(UDS, 0xB, "GetConnectionStatus", hdr(0xB, 0, 0), [], hdr(0xB, 13, 0), [0] + lost)
    log.call(UDS, 0xA, "DisconnectNetwork", hdr(0xA, 0, 0), [], hdr(0xA, 1, 0), [0])
    log.call(UDS, 0x3, "Shutdown", hdr(0x3, 0, 0), [], hdr(0x3, 1, 0), [0])
    return log


if __name__ == "__main__":
    host_log().write()
    client_log().write()
