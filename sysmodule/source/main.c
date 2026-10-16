// udsrelay sysmodule: answers every game's nwm::UDS over the relay (Phase 6).
//
// Luma3DS starts it at boot: the patched pm launches title 0004013000F00D02, and Luma's loader
// takes it from /luma/sysmodules/0004013000F00D02.cxi. It registers two services, and the
// patched sm hands them to any application that asks for the real ones:
//
//   uds:rly  in place of nwm::UDS: shim/core's emulator answers every request (relay mode).
//   ndm:rly  in place of ndm:u: forwarded to the real ndm:u, except the exclusive-state calls,
//            which it fakes so infrastructure Wi-Fi (and the relay) stays up during local play.
//
// A thread per session runs shim/core's session loop, as in the plugin. A relay thread starts
// sockets the first time a game opens uds:rly and then feeds the relay to the emulator, and a
// writer thread logs to sdmc:/udsrelay/logs/ in the plugin's UDSTRACE format.

#include <3ds.h>

#include <arpa/inet.h>
#include <errno.h>
#include <netdb.h>
#include <netinet/in.h>
#include <poll.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>

#include "shim/config.h"
#include "shim/ring.h"
#include "shim/session.h"
#include "shim/trace.h"
#include "shim/uds.h"

#define VERSION "0.4.0"

#define MAX_SESSIONS 4
#define SLOT_SIZE 0x800 // largest UDS static buffer is SendTo's 0x5F4
#define RING_SIZE (64 * 1024)
#define HEAP_SIZE (192 * 1024) // thread stacks and newlib's socket bookkeeping
#define SOC_SIZE 0x20000
#define WORKER_STACK 0x3000
#define WORKER_PRIORITY 0x20
#define RELAY_STACK 0x3000
#define RELAY_PRIORITY 0x20
#define WRITER_STACK 0x2000
#define WRITER_PRIORITY 0x30
#define NS_PER_MS 1000000LL
#define MS_FROM_1900_TO_1970 2208988800000ULL
#define CONFIG_PATH "/udsrelay/shim.ini"

// Luma3DS custom system call (csvc.s): svcControlMemory that can allocate from another region.
Result svcControlMemoryEx(u32 *addr_out, u32 addr0, u32 addr1, u32 size, MemOp op, MemPerm perm, bool isLoader);

// ---- startup ------------------------------------------------------------------------------
//
// A sysmodule must not start like an application: libctru's default initSystem maps every free
// byte of the memory region as heap and initialises applet and graphics services. This one
// follows Luma's Rosalina: a fixed heap inside the image, srv and fs only.

extern char *fake_heap_start, *fake_heap_end;
void __sync_init(void);
void __system_initSyscalls(void);
void __libc_init_array(void);

static u8 g_heap[HEAP_SIZE] __attribute__((aligned(0x1000)));

void __ctru_exit(int rc) { (void)rc; }

void __wrap_exit(int rc) {
    (void)rc;
    svcExitProcess();
}

void initSystem(void (*retAddr)(void)) {
    (void)retAddr;
    __sync_init();
    __system_initSyscalls();
    fake_heap_start = (char *)g_heap;
    fake_heap_end = (char *)g_heap + sizeof(g_heap);
    Result res;
    for (res = 0xD88007FA; res == (Result)0xD88007FA; svcSleepThread(500 * 1000LL)) {
        res = srvInit(); // sm may not be accepting sessions yet this early in boot
        if (R_FAILED(res) && res != (Result)0xD88007FA) {
            svcBreak(USERBREAK_PANIC);
        }
    }
    if (R_FAILED(fsInit())) {
        svcBreak(USERBREAK_PANIC);
    }
    __libc_init_array();
}

// ---- logging -------------------------------------------------------------------------------

static shim_config g_cfg;
static u64 g_baseTick, g_baseWallUs;
static LightLock g_logLock;
static shim_ring g_ring;
static u8 g_ringMem[RING_SIZE];
static char g_noteLine[1024]; // guarded by g_logLock

static shim_time Now(void) {
    const u64 delta = svcGetSystemTick() - g_baseTick;
    const u64 us = (delta / SYSCLOCK_ARM11) * 1000000ULL + (delta % SYSCLOCK_ARM11) * 1000000ULL / SYSCLOCK_ARM11;
    return (shim_time){g_baseWallUs + us, us};
}

static void LogLine(const char *line, size_t len) {
    if (!len) {
        return;
    }
    LightLock_Lock(&g_logLock);
    shim_ring_push(&g_ring, line, len);
    LightLock_Unlock(&g_logLock);
}

static void Note(const char *level, const char *func, const char *fmt, ...) __attribute__((format(printf, 3, 4)));
static void Note(const char *level, const char *func, const char *fmt, ...) {
    char msg[256];
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(msg, sizeof(msg), fmt, ap);
    va_end(ap);
    LightLock_Lock(&g_logLock);
    const size_t n = shim_trace_note(g_noteLine, sizeof(g_noteLine), Now(), level, func, msg);
    if (n) {
        shim_ring_push(&g_ring, g_noteLine, n);
    }
    LightLock_Unlock(&g_logLock);
}

static FS_Archive g_sdmc;

static void EnsureDir(const char *path) {
    FSUSER_CreateDirectory(g_sdmc, fsMakePath(PATH_ASCII, path), 0); // fails harmlessly if it exists
}

// Flushes the log to the SD card every second, one file per boot.
static void WriterThread(void *arg) {
    (void)arg;
    static char chunk[16 * 1024];
    char path[64];
    const time_t secs = (time_t)(g_baseWallUs / 1000000ULL);
    struct tm tm;
    gmtime_r(&secs, &tm);
    snprintf(path, sizeof(path), "/udsrelay/logs/sysmodule-%04d%02d%02d-%02d%02d%02d.log", tm.tm_year + 1900,
             tm.tm_mon + 1, tm.tm_mday, tm.tm_hour, tm.tm_min, tm.tm_sec);
    Handle file = 0;
    if (R_FAILED(FSUSER_OpenFile(&file, g_sdmc, fsMakePath(PATH_ASCII, path), FS_OPEN_WRITE | FS_OPEN_CREATE, 0))) {
        return; // nowhere to log; keep serving
    }
    u64 offset = 0, lastFlush = osGetTime();
    for (;;) {
        LightLock_Lock(&g_logLock);
        const size_t n = shim_ring_pop(&g_ring, chunk, sizeof(chunk));
        const u32 dropped = g_ring.dropped_lines;
        g_ring.dropped_lines = 0;
        LightLock_Unlock(&g_logLock);
        if (dropped) {
            Note("Warning", "Writer", "log ring full: dropped %lu lines", (unsigned long)dropped);
        }
        if (n) {
            u32 written = 0;
            FSFILE_Write(file, &written, offset, chunk, (u32)n, 0);
            offset += written;
        }
        if (osGetTime() - lastFlush >= 1000) {
            FSFILE_Flush(file);
            lastFlush = osGetTime();
        }
        if (!n) {
            svcSleepThread(200 * NS_PER_MS);
        }
    }
}

static void LoadConfig(void) {
    shim_config_defaults(&g_cfg);
    Handle file = 0;
    if (R_FAILED(FSUSER_OpenFile(&file, g_sdmc, fsMakePath(PATH_ASCII, CONFIG_PATH), FS_OPEN_READ, 0))) {
        return;
    }
    static char text[1024];
    u64 size = 0;
    u32 n = 0;
    FSFILE_GetSize(file, &size);
    if (size > sizeof(text) - 1) {
        size = sizeof(text) - 1;
    }
    if (R_SUCCEEDED(FSFILE_Read(file, &n, 0, text, (u32)size))) {
        const int bad = shim_config_parse(&g_cfg, text, n);
        if (bad) {
            Note("Warning", "LoadConfig", "%d unrecognised line(s) in %s", bad, CONFIG_PATH);
        }
    }
    FSFILE_Close(file);
}

// ---- the emulator's ops --------------------------------------------------------------------

static shim_uds g_uds;
static LightLock g_udsLock;
static CondVar g_udsCv;
static int g_relayFd = -1;
static struct sockaddr_in g_relayAddr;

static uint64_t UdsNow(void *ctx) {
    (void)ctx;
    return svcGetSystemTick() / (SYSCLOCK_ARM11 / 1000);
}
static void UdsLock(void *ctx) {
    (void)ctx;
    LightLock_Lock(&g_udsLock);
}
static void UdsUnlock(void *ctx) {
    (void)ctx;
    LightLock_Unlock(&g_udsLock);
}
static void UdsWait(void *ctx, uint32_t ms) {
    (void)ctx;
    CondVar_WaitTimeout(&g_udsCv, &g_udsLock, (s64)ms * NS_PER_MS);
}
static void UdsWake(void *ctx) {
    (void)ctx;
    CondVar_Broadcast(&g_udsCv);
}
static void UdsSend(void *ctx, const uint8_t *b, size_t n) {
    (void)ctx;
    if (g_relayFd >= 0) {
        sendto(g_relayFd, b, n, 0, (const struct sockaddr *)&g_relayAddr, sizeof(g_relayAddr));
    }
}
static shim_handle_t UdsCreateEvent(void *ctx) {
    (void)ctx;
    Handle h = 0;
    return R_SUCCEEDED(svcCreateEvent(&h, RESET_ONESHOT)) ? h : 0;
}
static void UdsSignal(void *ctx, shim_handle_t h) {
    (void)ctx;
    svcSignalEvent(h);
}
static void CloseHandleOp(void *ctx, shim_handle_t h) {
    (void)ctx;
    if (h) {
        svcCloseHandle(h);
    }
}
static uint32_t AddrOf(void *ctx, const void *p) {
    (void)ctx;
    return (uint32_t)p;
}
static uint8_t *UdsMem(void *ctx, uint32_t addr, uint32_t size) {
    (void)ctx, (void)size;
    return (uint8_t *)addr;
}
static void UdsLog(void *ctx, const char *level, const char *msg) {
    (void)ctx;
    Note(level, "UDS", "%s", msg);
}

static const shim_uds_ops g_udsOps = {NULL,           UdsNow,    UdsLock,      UdsUnlock,     UdsWait,
                                      UdsWake,        UdsSend,   UdsCreateEvent, UdsSignal,   CloseHandleOp,
                                      AddrOf,         UdsMem,    UdsLog};

// ---- session ops ---------------------------------------------------------------------------

static volatile u32 g_fakeExclusive; // the exclusive state the game believes it is in

static uint32_t *OpsCmdbuf(void *ctx) {
    (void)ctx;
    return getThreadCommandBuffer();
}
static uint32_t *OpsStaticBufs(void *ctx) {
    (void)ctx;
    return getThreadStaticBuffers();
}
static shim_result_t OpsSendSync(void *ctx, shim_handle_t h) {
    (void)ctx;
    return svcSendSyncRequest(h);
}
static shim_result_t OpsReplyAndReceive(void *ctx, int32_t *index, const shim_handle_t *handles, int32_t count,
                                        shim_handle_t replyTarget) {
    (void)ctx;
    s32 idx = -1;
    const Result rc = svcReplyAndReceive(&idx, handles, count, replyTarget);
    *index = idx;
    return rc;
}
static const uint8_t *OpsView(void *ctx, uint32_t addr, uint32_t size) {
    (void)ctx, (void)size;
    return (const uint8_t *)addr;
}
static shim_time OpsNow(void *ctx) {
    (void)ctx;
    return Now();
}
static void OpsLog(void *ctx, const char *line, size_t len) {
    (void)ctx;
    LogLine(line, len);
}

// nwm::UDS: every request goes to the emulator, with the session's reply bank as scratch for
// static-buffer replies. ndm:u: the exclusive-state calls are faked, everything else forwarded.
static int OpsAnswer(void *ctx, shim_session *s, uint32_t *cmd) {
    (void)ctx;
    if (strcmp(s->port, "nwm::UDS") == 0) {
        shim_uds_handle(&g_uds, cmd, s->bank_rsp);
        return 1;
    }
    switch (cmd[0] >> 16) {
    case 0x0001: // EnterExclusiveState(state, process ID)
        g_fakeExclusive = cmd[1];
        cmd[0] = IPC_MakeHeader(0x0001, 1, 0);
        cmd[1] = 0;
        return 1;
    case 0x0002: // LeaveExclusiveState(process ID)
        g_fakeExclusive = 0;
        cmd[0] = IPC_MakeHeader(0x0002, 1, 0);
        cmd[1] = 0;
        return 1;
    case 0x0003: // GetExclusiveState
        cmd[0] = IPC_MakeHeader(0x0003, 2, 0);
        cmd[1] = 0;
        cmd[2] = g_fakeExclusive;
        return 1;
    }
    return 0;
}

static const shim_ops g_ops = {NULL,          OpsCmdbuf, OpsStaticBufs, OpsSendSync, OpsReplyAndReceive,
                               CloseHandleOp, AddrOf,    OpsView,       OpsNow,      OpsLog,
                               OpsAnswer};

// ---- sessions ------------------------------------------------------------------------------

typedef struct {
    volatile bool used;
    bool ready; // worker thread running
    LightEvent start;
    char port[SHIM_PORT_NAME_MAX + 1];
    shim_session session;
    u8 bankReq[SHIM_STATIC_SLOTS * SLOT_SIZE];
    u8 bankRsp[SHIM_STATIC_SLOTS * SLOT_SIZE];
    char line[SHIM_TRACE_LINE_MAX];
} SessionSlot;

static SessionSlot g_slots[MAX_SESSIONS];
static LightLock g_slotLock;
static char g_proc[17] = "sysmodule";
static LightEvent g_relayWanted; // a game opened uds:rly: bring the relay up

static void SessionWorker(void *arg) {
    SessionSlot *slot = arg;
    for (;;) {
        LightEvent_Wait(&slot->start);
        const shim_result_t rc = shim_session_run(&slot->session, &g_ops);
        Note(rc ? "Error" : "Info", "SessionWorker", "%s session ended after %lu requests (%lu failed), result 0x%08lx",
             slot->port, (unsigned long)slot->session.requests, (unsigned long)slot->session.failures,
             (unsigned long)rc);
        LightLock_Lock(&g_slotLock);
        slot->used = false;
        LightLock_Unlock(&g_slotLock);
    }
}

// Hands an accepted session to a free worker. real is the real service session (ndm:u), or 0.
static void Dispatch(const char *port, Handle server, Handle real, int index) {
    LightLock_Lock(&g_slotLock);
    SessionSlot *slot = NULL;
    for (int i = 0; i < MAX_SESSIONS && !slot; i++) {
        if (g_slots[i].ready && !g_slots[i].used) {
            slot = &g_slots[i];
        }
    }
    if (!slot) {
        LightLock_Unlock(&g_slotLock);
        Note("Warning", "Dispatch", "no free session slot for %s; closing it", port);
        svcCloseHandle(server);
        if (real) {
            svcCloseHandle(real);
        }
        return;
    }
    strncpy(slot->port, port, SHIM_PORT_NAME_MAX);
    slot->port[SHIM_PORT_NAME_MAX] = '\0';
    memset(&slot->session, 0, sizeof(slot->session));
    slot->session.port = slot->port;
    slot->session.sess = index;
    slot->session.server = server;
    slot->session.real = real;
    slot->session.proc = g_proc;
    slot->session.payload_max = g_cfg.payload_max;
    slot->session.bank_req = slot->bankReq;
    slot->session.bank_rsp = slot->bankRsp;
    slot->session.slot_size = SLOT_SIZE;
    slot->session.line = slot->line;
    slot->used = true;
    LightLock_Unlock(&g_slotLock);
    LightEvent_Signal(&slot->start);
    Note("Info", "Dispatch", "serving a %s session in slot %d", port, (int)(slot - g_slots));
}

// ---- relay thread --------------------------------------------------------------------------

static bool SocInitSystemRegion(void) {
    const MemPerm rw = (MemPerm)(MEMPERM_READ | MEMPERM_WRITE);
    u32 buf = 0;
    // As Rosalina does: from the SYSTEM region, so the socket buffer does not come out of the
    // small BASE region system modules share.
    Result rc = svcControlMemoryEx(&buf, 0x08000000, 0, SOC_SIZE, (MemOp)(MEMOP_ALLOC | MEMOP_REGION_SYSTEM), rw, true);
    if (R_FAILED(rc)) {
        Note("Warning", "Relay", "SYSTEM-region socket buffer refused (0x%08lx); trying our own region",
             (unsigned long)rc);
        rc = svcControlMemory(&buf, 0x08000000, 0, SOC_SIZE, MEMOP_ALLOC, rw);
        if (R_FAILED(rc)) {
            Note("Error", "Relay", "cannot allocate a socket buffer: 0x%08lx", (unsigned long)rc);
            return false;
        }
    }
    rc = socInit((u32 *)buf, SOC_SIZE);
    if (R_FAILED(rc)) {
        Note("Error", "Relay", "socInit failed 0x%08lx", (unsigned long)rc);
        return false;
    }
    Note("Info", "Relay", "socInit ok with a buffer at 0x%08lx", (unsigned long)buf);
    return true;
}

static bool ResolveRelay(void) {
    memset(&g_relayAddr, 0, sizeof(g_relayAddr));
    g_relayAddr.sin_family = AF_INET;
    g_relayAddr.sin_port = htons(g_cfg.relay_port);
    if (inet_aton(g_cfg.relay_host, &g_relayAddr.sin_addr)) {
        return true;
    }
    struct hostent *he = gethostbyname(g_cfg.relay_host);
    if (!he || he->h_addrtype != AF_INET || !he->h_addr_list[0]) {
        return false;
    }
    memcpy(&g_relayAddr.sin_addr, he->h_addr_list[0], sizeof(g_relayAddr.sin_addr));
    return true;
}

static void RelayThread(void *arg) {
    (void)arg;
    LightEvent_Wait(&g_relayWanted);
    while (!SocInitSystemRegion()) {
        svcSleepThread(5000 * NS_PER_MS);
    }
    const int fd = socket(AF_INET, SOCK_DGRAM, 0);
    if (fd < 0) {
        Note("Error", "Relay", "socket() failed, errno %d", errno);
        return;
    }
    while (!ResolveRelay()) {
        Note("Warning", "Relay", "cannot resolve %s; retrying in 5 s", g_cfg.relay_host);
        svcSleepThread(5000 * NS_PER_MS);
    }
    LightLock_Lock(&g_udsLock);
    g_relayFd = fd;
    LightLock_Unlock(&g_udsLock);
    Note("Info", "Relay", "relay mode: %s:%u", g_cfg.relay_host, (unsigned)g_cfg.relay_port);
    static u8 buf[SHIM_RELAY_MAX_DATAGRAM];
    u64 lastPoll = 0;
    for (;;) {
        struct pollfd pfd = {fd, POLLIN, 0};
        if (poll(&pfd, 1, 20) > 0 && (pfd.revents & POLLIN)) {
            const ssize_t n = recv(fd, buf, sizeof(buf), 0);
            if (n > 0) {
                shim_uds_net_input(&g_uds, buf, (size_t)n);
            }
        }
        if (UdsNow(NULL) - lastPoll >= 20) {
            shim_uds_net_poll(&g_uds);
            lastPoll = UdsNow(NULL);
        }
    }
}

// ---- main ----------------------------------------------------------------------------------

int main(void) {
    g_baseTick = svcGetSystemTick();
    g_baseWallUs = (osGetTime() - MS_FROM_1900_TO_1970) * 1000ULL;
    LightLock_Init(&g_logLock);
    LightLock_Init(&g_slotLock);
    LightLock_Init(&g_udsLock);
    CondVar_Init(&g_udsCv);
    LightEvent_Init(&g_relayWanted, RESET_STICKY);
    shim_ring_init(&g_ring, g_ringMem, sizeof(g_ringMem));

    if (R_FAILED(FSUSER_OpenArchive(&g_sdmc, ARCHIVE_SDMC, fsMakePath(PATH_EMPTY, "")))) {
        svcBreak(USERBREAK_PANIC);
    }
    EnsureDir("/udsrelay");
    EnsureDir("/udsrelay/logs");
    threadCreate(WriterThread, NULL, WRITER_STACK, WRITER_PRIORITY, -2, true);
    LoadConfig();

    if (!g_cfg.relay_host[0] || !g_cfg.lobby[0]) {
        // Without the services, sm gives games the real ones: local wireless works as usual.
        Note("Info", "Start", "udsrelay sysmodule %s: no relay or lobby in %s; not serving", VERSION, CONFIG_PATH);
        svcSleepThread(2000 * NS_PER_MS); // let the writer flush
        return 0;
    }
    const u32 session = ((u32)svcGetSystemTick() ^ (u32)(svcGetSystemTick() >> 32) * 2654435761u) | 1;
    if (shim_uds_init(&g_uds, &g_udsOps, g_cfg.lobby, strlen(g_cfg.lobby), session) != 0) {
        Note("Error", "Start", "bad lobby key in %s; not serving", CONFIG_PATH);
        svcSleepThread(2000 * NS_PER_MS);
        return 0;
    }

    for (int i = 0; i < MAX_SESSIONS; i++) {
        LightEvent_Init(&g_slots[i].start, RESET_ONESHOT);
        g_slots[i].ready = threadCreate(SessionWorker, &g_slots[i], WORKER_STACK, WORKER_PRIORITY, -2, true) != NULL;
    }
    if (!threadCreate(RelayThread, NULL, RELAY_STACK, RELAY_PRIORITY, -2, true)) {
        Note("Error", "Start", "could not start the relay thread; not serving");
        svcSleepThread(2000 * NS_PER_MS);
        return 0;
    }

    Handle ports[2] = {0, 0};
    if (R_FAILED(srvRegisterService(&ports[0], "uds:rly", MAX_SESSIONS)) ||
        R_FAILED(srvRegisterService(&ports[1], "ndm:rly", MAX_SESSIONS))) {
        Note("Error", "Start", "could not register uds:rly and ndm:rly; games keep the real services");
        svcSleepThread(2000 * NS_PER_MS);
        return 0;
    }
    Note("Info", "Start", "udsrelay sysmodule %s serving uds:rly and ndm:rly; payload_max %ld", VERSION,
         g_cfg.payload_max);

    int next = 0;
    for (;;) {
        s32 idx = -1;
        if (R_FAILED(svcWaitSynchronizationN(&idx, ports, 2, false, U64_MAX)) || idx < 0 || idx > 1) {
            svcSleepThread(100 * NS_PER_MS);
            continue;
        }
        Handle server = 0;
        if (R_FAILED(svcAcceptSession(&server, ports[idx]))) {
            continue;
        }
        if (idx == 0) {
            LightEvent_Signal(&g_relayWanted);
            Dispatch("nwm::UDS", server, 0, next++);
        } else {
            Handle real = 0;
            // sm gives the sysmodule itself the real ndm:u.
            const Result rc = srvGetServiceHandle(&real, "ndm:u");
            if (R_FAILED(rc)) {
                Note("Error", "Main", "cannot open the real ndm:u: 0x%08lx", (unsigned long)rc);
                svcCloseHandle(server);
                continue;
            }
            Dispatch("ndm:u", server, real, next++);
        }
    }
}
