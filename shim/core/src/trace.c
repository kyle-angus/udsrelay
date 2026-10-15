#include "shim/trace.h"

#include <stdarg.h>
#include <stdio.h>
#include <string.h>

typedef struct {
    char *buf;
    size_t cap;
    size_t len;
    int overflow;
} sbuf;

static void sb_printf(sbuf *b, const char *fmt, ...) {
    if (b->overflow) {
        return;
    }
    va_list ap;
    va_start(ap, fmt);
    const int n = vsnprintf(b->buf + b->len, b->cap - b->len, fmt, ap);
    va_end(ap);
    if (n < 0 || (size_t)n >= b->cap - b->len) {
        b->overflow = 1;
        return;
    }
    b->len += (size_t)n;
}

static void sb_hex(sbuf *b, const uint8_t *data, size_t n) {
    static const char digits[] = "0123456789abcdef";
    if (b->overflow) {
        return;
    }
    if (b->cap - b->len <= 2 * n) {
        b->overflow = 1;
        return;
    }
    for (size_t i = 0; i < n; i++) {
        b->buf[b->len++] = digits[data[i] >> 4];
        b->buf[b->len++] = digits[data[i] & 0xF];
    }
    b->buf[b->len] = '\0';
}

static size_t sb_finish(sbuf *b) {
    sb_printf(b, "\n");
    return b->overflow ? 0 : b->len;
}

// "[  12.345678] Service <Info> shim:Forward:0: " mirrors Azahar's prefix so the same regexes work.
static void prefix(sbuf *b, shim_time t, const char *cls, const char *level, const char *func) {
    sb_printf(b, "[%4u.%06u] %s <%s> shim:%s:0: ", (unsigned)(t.uptime_us / 1000000u),
              (unsigned)(t.uptime_us % 1000000u), cls, level, func);
}

size_t shim_trace_msg(char *out, size_t cap, shim_time t, const char *kind, const char *port, int sess,
                      uint16_t cmd, const char *name, uint32_t tid, const char *proc,
                      const uint32_t *words, int deferred) {
    sbuf b = {out, cap, 0, 0};
    const unsigned n = shim_ipc_words(words[0]) > SHIM_IPC_MAX_WORDS ? SHIM_IPC_MAX_WORDS
                                                                      : shim_ipc_words(words[0]);
    prefix(&b, t, "Service", "Info", "Forward");
    sb_printf(&b, "UDSTRACE v=1 kind=%s wall=%llu port=%s", kind, (unsigned long long)t.wall_us, port);
    if (sess >= 0) {
        sb_printf(&b, " sess=%d", sess);
    }
    sb_printf(&b, " cmd=0x%04x", cmd);
    if (strcmp(kind, "req") == 0) {
        sb_printf(&b, " name=\"%s\" tid=%u proc=\"%s\"", name ? name : "", (unsigned)tid,
                  proc ? proc : "");
    }
    sb_printf(&b, " hdr=0x%08x", (unsigned)words[0]);
    if (strcmp(kind, "rsp") == 0) {
        sb_printf(&b, " deferred=%d", deferred ? 1 : 0);
    }
    sb_printf(&b, " words=");
    for (unsigned i = 0; i < n; i++) {
        sb_printf(&b, i ? ",0x%08x" : "0x%08x", (unsigned)words[i]);
    }
    return sb_finish(&b);
}

size_t shim_trace_blob(char *out, size_t cap, shim_time t, const char *port, int sess, uint16_t cmd,
                       const char *tag, const char *extra, const uint8_t *data, size_t size,
                       size_t logged) {
    sbuf b = {out, cap, 0, 0};
    if (logged > size) {
        logged = size;
    }
    prefix(&b, t, "Service", "Info", "TraceBlob");
    sb_printf(&b, "UDSTRACE v=1 kind=blob wall=%llu port=%s", (unsigned long long)t.wall_us, port);
    if (sess >= 0) {
        sb_printf(&b, " sess=%d", sess);
    }
    sb_printf(&b, " cmd=0x%04x tag=%s", cmd, tag);
    if (extra && *extra) {
        sb_printf(&b, " %s", extra);
    }
    sb_printf(&b, " size=%u hex=", (unsigned)size);
    sb_hex(&b, data, logged);
    return sb_finish(&b);
}

size_t shim_trace_note(char *out, size_t cap, shim_time t, const char *level, const char *func,
                       const char *message) {
    sbuf b = {out, cap, 0, 0};
    prefix(&b, t, "Plugin", level, func);
    sb_printf(&b, "%s", message);
    return sb_finish(&b);
}

const char *shim_trace_tag(const char *port, uint16_t cmd, int reply, uint8_t id, int mapped) {
    if (strcmp(port, "nwm::UDS") != 0) {
        return NULL;
    }
    if (mapped) {
        // RecvBeaconBroadcastData / ScanOnConnection: the game's buffer NWM fills with beacons.
        return (reply && (cmd == 0x000F || cmd == 0x0022)) ? "beacons" : NULL;
    }
    if (!reply) {
        switch (cmd) {
        case 0x0004: // BeginHostingNetwork (deprecated): network struct travels in words
        case 0x0009: // ConnectToNetwork (deprecated)
            return id == 0 ? "passphrase" : NULL;
        case 0x001D: // BeginHostingNetwork
        case 0x001E: // ConnectToNetwork
            return id == 1 ? "netinfo" : id == 0 ? "passphrase" : NULL;
        case 0x0010:
            return id == 4 ? "appdata" : NULL;
        case 0x0017:
            return id == 5 ? "send" : NULL;
        case 0x001F: // DecryptBeaconData: inputs are a scanned network, not ours
        case 0x000E:
            return id == 1 ? "beacon_netinfo" : id == 2 ? "beacon_tag0" : id == 3 ? "beacon_tag1" : NULL;
        }
        return NULL;
    }
    switch (cmd) {
    case 0x0014:
        return id == 0 ? "recv" : NULL;
    case 0x0011:
        return id == 0 ? "appdata_out" : NULL;
    case 0x001F:
    case 0x000E:
        return id == 0 ? "beacon_nodes" : NULL;
    }
    return NULL;
}

typedef struct {
    uint16_t cmd;
    const char *name;
} cmd_name;

static const cmd_name uds_names[] = {
    {0x0001, "Initialize (deprecated)"},
    {0x0003, "Shutdown"},
    {0x0004, "BeginHostingNetwork (deprecated)"},
    {0x0005, "EjectClient"},
    {0x0006, "EjectSpectators"},
    {0x0007, "UpdateNetworkAttribute"},
    {0x0008, "DestroyNetwork"},
    {0x0009, "ConnectToNetwork (deprecated)"},
    {0x000A, "DisconnectNetwork"},
    {0x000B, "GetConnectionStatus"},
    {0x000D, "GetNodeInformation"},
    {0x000E, "DecryptBeaconData (deprecated)"},
    {0x000F, "RecvBeaconBroadcastData"},
    {0x0010, "SetApplicationData"},
    {0x0011, "GetApplicationData"},
    {0x0012, "Bind"},
    {0x0013, "Unbind"},
    {0x0014, "PullPacket"},
    {0x0015, "SetMaxSendDelay"},
    {0x0017, "SendTo"},
    {0x001A, "GetChannel"},
    {0x001B, "InitializeWithVersion"},
    {0x001D, "BeginHostingNetwork"},
    {0x001E, "ConnectToNetwork"},
    {0x001F, "DecryptBeaconData"},
    {0x0020, "Flush"},
    {0x0021, "SetProbeResponseParam"},
    {0x0022, "ScanOnConnection"},
};

static const cmd_name ndm_names[] = {
    {0x0001, "EnterExclusiveState"},
    {0x0002, "LeaveExclusiveState"},
    {0x0003, "QueryExclusiveMode"},
    {0x0004, "LockState"},
    {0x0005, "UnlockState"},
    {0x0006, "SuspendDaemons"},
    {0x0007, "ResumeDaemons"},
    {0x0008, "SuspendScheduler"},
    {0x0009, "ResumeScheduler"},
    {0x000A, "GetCurrentState"},
    {0x000B, "GetTargetState"},
    {0x000D, "QueryStatus"},
    {0x000E, "GetDaemonDisableCount"},
    {0x000F, "GetSchedulerDisableCount"},
    {0x0010, "SetScanInterval"},
    {0x0011, "GetScanInterval"},
    {0x0012, "SetRetryInterval"},
    {0x0013, "GetRetryInterval"},
    {0x0014, "OverrideDefaultDaemons"},
    {0x0015, "ResetDefaultDaemons"},
    {0x0016, "GetDefaultDaemons"},
    {0x0017, "ClearHalfAwakeMacFilter"},
};

const char *shim_command_name(const char *port, uint16_t cmd) {
    const cmd_name *table = NULL;
    size_t n = 0;
    if (strcmp(port, "nwm::UDS") == 0) {
        table = uds_names;
        n = sizeof(uds_names) / sizeof(uds_names[0]);
    } else if (strcmp(port, "ndm:u") == 0) {
        table = ndm_names;
        n = sizeof(ndm_names) / sizeof(ndm_names[0]);
    }
    for (size_t i = 0; i < n; i++) {
        if (table[i].cmd == cmd) {
            return table[i].name;
        }
    }
    return NULL;
}
