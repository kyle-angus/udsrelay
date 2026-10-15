// Phase 2 passthrough shim: console glue around shim/core.
//
// PatchProcess (before the game's first instruction) finds the game's srv:GetServiceHandle
// client function and MITM-hooks it. When the game obtains a traced service (nwm::UDS, ndm:u
// by default), the hook swaps the handle it returns for the client end of a session pair the
// shim serves; a thread per session forwards every request to the real service and logs it.
// Logs go to the SD card, since infrastructure Wi-Fi is down while real local wireless runs.

#include <3ds.h>
#include <CTRPluginFramework.hpp>

#include <cstdarg>
#include <cstdio>
#include <cstring>
#include <ctime>

#include "shim/config.h"
#include "shim/finder.h"
#include "shim/ring.h"
#include "shim/session.h"
#include "shim/trace.h"

#include "luma_svc.hpp"
#include "shim_plugin.hpp"

using namespace CTRPluginFramework;

namespace udsrelay {

namespace {

constexpr u32 kTextBase = 0x00100000;
constexpr int kMaxHooks = 4;
constexpr int kMaxSessions = 4;
constexpr size_t kSlotSize = 0x800; // largest UDS static buffer is SendTo's 0x5F4
constexpr size_t kRingSize = 256 * 1024;
constexpr u32 kSessionStack = 0x4000;
constexpr s32 kSessionPriority = 0x19; // above typical game threads so forwarding is prompt
constexpr u32 kWriterStack = 0x4000;
constexpr s32 kWriterPriority = 0x30;
constexpr u64 kNsPerMs = 1000000ULL;
constexpr u64 kMsFrom1900To1970 = 2208988800000ULL;
constexpr const char *kDir = "/udsrelay";
constexpr const char *kLogDir = "/udsrelay/logs";
constexpr const char *kConfigPath = "/udsrelay/shim.ini";

// Each slot has a worker thread started by Start(). The GetServiceHandle hook runs on the
// game's own threads, where libctru's threadCreate faults (it copies stdio pointers from the
// caller's libctru thread vars, which game threads lack), so the hook only fills in a slot and
// signals its worker.
struct SessionSlot {
    volatile bool used;
    bool ready; // worker thread running
    LightEvent start;
    char port[SHIM_PORT_NAME_MAX + 1];
    shim_session session;
    u8 bankReq[SHIM_STATIC_SLOTS * kSlotSize];
    u8 bankRsp[SHIM_STATIC_SLOTS * kSlotSize];
    char line[SHIM_TRACE_LINE_MAX];
};

shim_config g_cfg;
char g_proc[17];
u64 g_baseWallUs;
u64 g_baseTick;

LightLock g_logLock;
shim_ring g_ring;
u8 g_ringMem[kRingSize];
char g_noteLine[1024]; // guarded by g_logLock

Hook g_hooks[kMaxHooks];
int g_hookCount;

LightLock g_slotLock;
SessionSlot g_slots[kMaxSessions];
Handle g_attachedReal[kMaxSessions]; // real handles already intercepted (guarded by g_slotLock)

// The writer parks (blocked in the kernel, touching no memory) while the game is in the HOME
// menu, swapped out, asleep or exiting: Luma may save and scramble plugin memory then, and
// writes made meanwhile come back as garbage. The log is flushed every second, so exit does no
// file I/O; the kernel closes the file with the process.
enum WriterState : u32 { WRITER_RUN, WRITER_PARK };
volatile u32 g_writerWant = WRITER_RUN;
volatile bool g_writerAlive;
LightEvent g_writerIdle;   // writer reached the state g_writerWant asked for
LightEvent g_writerResume; // wakes a parked writer

volatile u32 g_attachCount;
volatile u32 g_activeSessions;
volatile u32 g_bytesWritten;
volatile u32 g_droppedLines;
char g_logPath[64];

shim_time Now() {
    const u64 delta = svcGetSystemTick() - g_baseTick;
    const u64 us = (delta / SYSCLOCK_ARM11) * 1000000ULL + (delta % SYSCLOCK_ARM11) * 1000000ULL / SYSCLOCK_ARM11;
    return shim_time{g_baseWallUs + us, us};
}

void PushLocked(const char *line, size_t len) {
    if (len) {
        shim_ring_push(&g_ring, line, len);
    }
}

void LogLine(const char *line, size_t len) {
    LightLock_Lock(&g_logLock);
    PushLocked(line, len);
    LightLock_Unlock(&g_logLock);
}

void Note(const char *level, const char *func, const char *fmt, ...) __attribute__((format(printf, 3, 4)));
void Note(const char *level, const char *func, const char *fmt, ...) {
    char msg[256];
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(msg, sizeof(msg), fmt, ap);
    va_end(ap);
    LightLock_Lock(&g_logLock);
    PushLocked(g_noteLine, shim_trace_note(g_noteLine, sizeof(g_noteLine), Now(), level, func, msg));
    LightLock_Unlock(&g_logLock);
}

// ---- shim_ops on the console ---------------------------------------------------------------

u32 *OpsCmdbuf(void *) { return getThreadCommandBuffer(); }
u32 *OpsStaticBufs(void *) { return getThreadStaticBuffers(); }
shim_result_t OpsSendSync(void *, shim_handle_t h) { return svcSendSyncRequest(h); }
shim_result_t OpsReplyAndReceive(void *, int32_t *index, const shim_handle_t *handles, int32_t count,
                                 shim_handle_t replyTarget) {
    s32 idx = -1;
    const Result rc = svcReplyAndReceive(&idx, handles, count, replyTarget);
    *index = idx;
    return rc;
}
void OpsClose(void *, shim_handle_t h) { svcCloseHandle(h); }
uint32_t OpsAddrOf(void *, const void *p) { return reinterpret_cast<u32>(p); }
const uint8_t *OpsView(void *, uint32_t addr, uint32_t) { return reinterpret_cast<const u8 *>(addr); }
shim_time OpsNow(void *) { return Now(); }
void OpsLog(void *, const char *line, size_t len) { LogLine(line, len); }

const shim_ops g_ops = {nullptr,  OpsCmdbuf, OpsStaticBufs, OpsSendSync, OpsReplyAndReceive,
                        OpsClose, OpsAddrOf, OpsView,       OpsNow,      OpsLog};

// ---- sessions -------------------------------------------------------------------------------

// Waits for Attach to hand this slot a session, forwards it until the game closes it, then
// frees the slot and waits again.
void SessionWorker(void *arg) {
    SessionSlot *slot = static_cast<SessionSlot *>(arg);
    for (;;) {
        LightEvent_Wait(&slot->start);
        __atomic_add_fetch(&g_activeSessions, 1, __ATOMIC_SEQ_CST);
        const shim_result_t rc = shim_session_run(&slot->session, &g_ops);
        Note(rc ? "Error" : "Info", "SessionWorker",
             "%s session ended after %lu requests (%lu failed), result 0x%08lx", slot->port,
             (unsigned long)slot->session.requests, (unsigned long)slot->session.failures, (unsigned long)rc);
        __atomic_sub_fetch(&g_activeSessions, 1, __ATOMIC_SEQ_CST);
        LightLock_Lock(&g_slotLock);
        for (int i = 0; i < kMaxSessions; i++) {
            if (&g_slots[i] == slot) {
                g_attachedReal[i] = 0;
            }
        }
        slot->used = false;
        LightLock_Unlock(&g_slotLock);
    }
}

// Starts one worker per slot. Runs from PatchProcess, on a thread libctru set up.
void StartWorkers() {
    int started = 0;
    for (int i = 0; i < kMaxSessions; i++) {
        SessionSlot *slot = &g_slots[i];
        LightEvent_Init(&slot->start, RESET_ONESHOT);
        slot->ready = threadCreate(SessionWorker, slot, kSessionStack, kSessionPriority, -2, true) != nullptr;
        started += slot->ready;
    }
    if (started < kMaxSessions) {
        Note("Error", "StartWorkers", "started %d of %d session workers", started, kMaxSessions);
    }
}

// Creates a session pair for `real` and wakes the slot's worker to forward it. On success the
// game gets *client in place of `real`. Called from the game's threads: no threadCreate here.
bool Attach(const char *name, Handle real, Handle *client) {
    LightLock_Lock(&g_slotLock);
    int idx = -1;
    for (int i = 0; i < kMaxSessions; i++) {
        if (g_slots[i].used && g_attachedReal[i] == real) {
            LightLock_Unlock(&g_slotLock);
            return false; // already intercepted (nested GetServiceHandle hooks)
        }
        if (g_slots[i].ready && !g_slots[i].used && idx < 0) {
            idx = i;
        }
    }
    if (idx < 0) {
        LightLock_Unlock(&g_slotLock);
        Note("Warning", "Attach", "no free session slot for %s; passing the real handle through", name);
        return false;
    }
    SessionSlot *slot = &g_slots[idx];
    Handle server = 0;
    const Result rc = svcCreateSession(&server, client);
    if (R_FAILED(rc)) {
        LightLock_Unlock(&g_slotLock);
        Note("Error", "Attach", "svcCreateSession failed 0x%08lx", (unsigned long)rc);
        return false;
    }
    std::strncpy(slot->port, name, SHIM_PORT_NAME_MAX);
    slot->port[SHIM_PORT_NAME_MAX] = '\0';
    std::memset(&slot->session, 0, sizeof(slot->session));
    slot->session.port = slot->port;
    slot->session.sess = idx;
    slot->session.server = server;
    slot->session.real = real;
    slot->session.proc = g_proc;
    slot->session.payload_max = g_cfg.payload_max;
    slot->session.bank_req = slot->bankReq;
    slot->session.bank_rsp = slot->bankRsp;
    slot->session.slot_size = kSlotSize;
    slot->session.line = slot->line;
    slot->used = true;
    g_attachedReal[idx] = real;
    LightLock_Unlock(&g_slotLock);
    LightEvent_Signal(&slot->start);
    __atomic_add_fetch(&g_attachCount, 1, __ATOMIC_SEQ_CST);
    Note("Info", "Attach", "intercepted %s: real 0x%08lx, game now holds 0x%08lx", name, (unsigned long)real,
         (unsigned long)*client);
    return true;
}

// Valid, writable, aligned word in this process.
int ReadWritableWord(void *, uint32_t addr, uint32_t *value) {
    MemInfo mi;
    PageInfo pi;
    if (R_FAILED(svcQueryMemory(&mi, &pi, addr)) || mi.state == MEMSTATE_FREE ||
        !(mi.perm & MEMPERM_WRITE) || !(mi.perm & MEMPERM_READ)) {
        return -1;
    }
    *value = *reinterpret_cast<const u32 *>(addr);
    return 0;
}

// MITM replacement for the game's GetServiceHandle. The SDK function takes at most four
// register arguments; one of them is the Handle* it fills in.
Result OnGetServiceHandle(u32 a0, u32 a1, u32 a2, u32 a3) {
    const Result res = HookContext::GetCurrent().OriginalFunction<Result>(a0, a1, a2, a3);
    if (R_FAILED(res)) {
        return res;
    }
    // srv:GetServiceHandle reply still in TLS: header (5,1,2), result, move descriptor, handle.
    const u32 *cmd = getThreadCommandBuffer();
    if (cmd[0] != 0x00050042 || cmd[1] != 0) {
        return res;
    }
    const Handle real = cmd[3];
    char name[12] = {0};
    if (R_FAILED(svcControlService(SERVICEOP_GET_NAME, name, real))) {
        return res;
    }
    if (!shim_config_traces(&g_cfg, name)) {
        return res;
    }
    const u32 args[4] = {a0, a1, a2, a3};
    const int out = shim_pick_out_arg(args, real, ReadWritableWord, nullptr);
    if (out < 0) {
        Note("Warning", "OnGetServiceHandle",
             "%s: could not tell which argument is the output handle (r0-r3 = %08lx %08lx %08lx %08lx)", name,
             (unsigned long)a0, (unsigned long)a1, (unsigned long)a2, (unsigned long)a3);
        return res;
    }
    Handle client = 0;
    if (Attach(name, real, &client)) {
        *reinterpret_cast<Handle *>(args[out]) = client;
    }
    return res;
}

// ---- log writer -----------------------------------------------------------------------------

void WriterThread(void *) {
    static char chunk[16 * 1024];
    File file;
    if (File::Open(file, g_logPath, File::WRITE | File::CREATE | File::TRUNCATE) != File::SUCCESS) {
        OSD::Notify(Utils::Format("UDS shim: cannot open %s; not logging", g_logPath), Color::Red);
        return;
    }
    g_writerAlive = true;
    u32 lastAttach = 0;
    u64 lastFlush = osGetTime();
    for (;;) {
        const u32 want = g_writerWant;
        if (want == WRITER_PARK) {
            file.Flush();
            LightEvent_Signal(&g_writerIdle);
            LightEvent_Wait(&g_writerResume);
            continue;
        }
        LightLock_Lock(&g_logLock);
        const size_t n = shim_ring_pop(&g_ring, chunk, sizeof(chunk));
        const u32 dropped = g_ring.dropped_lines;
        g_ring.dropped_lines = 0;
        LightLock_Unlock(&g_logLock);

        if (dropped) {
            g_droppedLines += dropped;
            Note("Warning", "Writer", "log ring full: dropped %lu lines", (unsigned long)dropped);
        }
        if (n) {
            file.Write(chunk, n);
            g_bytesWritten += n;
        }
        if (osGetTime() - lastFlush >= 1000) {
            file.Flush();
            lastFlush = osGetTime();
        }
        if (g_attachCount != lastAttach) {
            lastAttach = g_attachCount;
            OSD::Notify(Utils::Format("UDS shim: %lu session(s) intercepted", (unsigned long)lastAttach));
        }
        if (!n) {
            svcSleepThread(50 * kNsPerMs);
        }
    }
}

// Asks the writer to park or run. Parking waits at most 100 ms (the writer polls every 50 ms):
// this runs inside CTRPF's event callback, which Luma expects to answer promptly.
void SetWriter(WriterState want) {
    if (!g_writerAlive || g_writerWant == want) {
        return;
    }
    LightEvent_Clear(&g_writerIdle);
    g_writerWant = want;
    if (want == WRITER_RUN) {
        LightEvent_Signal(&g_writerResume);
    } else {
        LightEvent_WaitTimeout(&g_writerIdle, 100 * kNsPerMs);
    }
}

void OnProcessEvent(Process::Event event) {
    switch (event) {
    case Process::Event::HOME_EXIT:
    case Process::Event::SWAP_EXIT:
    case Process::Event::SLEEP_EXIT:
        SetWriter(WRITER_RUN);
        break;
    default: // HOME_ENTER, SWAP_ENTER, SLEEP_ENTER, EXIT
        SetWriter(WRITER_PARK);
        break;
    }
}

void LoadConfig() {
    shim_config_defaults(&g_cfg);
    File file;
    if (File::Open(file, kConfigPath, File::READ) != File::SUCCESS) {
        return;
    }
    char text[1024];
    const u64 size = file.GetSize();
    const u32 n = size < sizeof(text) - 1 ? static_cast<u32>(size) : sizeof(text) - 1;
    // File::Read returns a status (OPResult or FS error), not a byte count.
    if (n > 0 && file.Read(text, n) == File::SUCCESS) {
        const int bad = shim_config_parse(&g_cfg, text, n);
        if (bad) {
            Note("Warning", "LoadConfig", "%d unrecognised line(s) in %s", bad, kConfigPath);
        }
    }
}

void InitClock() {
    g_baseTick = svcGetSystemTick();
    g_baseWallUs = (osGetTime() - kMsFrom1900To1970) * 1000ULL;
}

void MakeLogPath() {
    const time_t unixSeconds = static_cast<time_t>(g_baseWallUs / 1000000ULL);
    struct tm tm;
    gmtime_r(&unixSeconds, &tm);
    snprintf(g_logPath, sizeof(g_logPath), "%s/%s-%04d%02d%02d-%02d%02d%02d.log", kLogDir, g_proc,
             tm.tm_year + 1900, tm.tm_mon + 1, tm.tm_mday, tm.tm_hour, tm.tm_min, tm.tm_sec);
}

void InstallHooks() {
    const u32 textSize = Process::GetTextSize();
    shim_hook_candidate found[kMaxHooks];
    const size_t n = shim_find_get_service_handle(reinterpret_cast<const u32 *>(kTextBase), textSize / 4,
                                                  kTextBase, found, kMaxHooks);
    if (n == 0) {
        Note("Error", "InstallHooks", "no srv:GetServiceHandle function found in %lu bytes of .text",
             (unsigned long)textSize);
        return;
    }
    for (size_t i = 0; i < n; i++) {
        Hook &hook = g_hooks[g_hookCount];
        hook.InitializeForMitm(found[i].func, reinterpret_cast<u32>(OnGetServiceHandle));
        const HookResult hr = hook.Enable();
        Note(hr == HookResult::Success ? "Info" : "Error", "InstallHooks",
             "GetServiceHandle candidate at 0x%08lx (literal 0x%08lx, svc 0x%08lx): hook result %d",
             (unsigned long)found[i].func, (unsigned long)found[i].literal, (unsigned long)found[i].svc,
             static_cast<int>(hr));
        if (hr == HookResult::Success) {
            g_hookCount++;
        }
    }
}

} // namespace

void Start() {
    LightLock_Init(&g_logLock);
    LightLock_Init(&g_slotLock);
    shim_ring_init(&g_ring, g_ringMem, sizeof(g_ringMem));
    InitClock();
    snprintf(g_proc, sizeof(g_proc), "%016llX", Process::GetTitleID());

    if (!Directory::IsExists(kDir)) {
        Directory::Create(kDir);
    }
    if (!Directory::IsExists(kLogDir)) {
        Directory::Create(kLogDir);
    }
    LoadConfig();
    MakeLogPath();

    char ports[64] = {0};
    for (int i = 0; i < g_cfg.nports; i++) {
        if (i) {
            std::strcat(ports, ",");
        }
        std::strcat(ports, g_cfg.ports[i]);
    }
    Note("Info", "Start", "udsrelay shim %s for title %s; tracing %s; payload_max %ld", kShimVersion, g_proc, ports,
         g_cfg.payload_max);

    StartWorkers(); // before the hooks, so a slot is ready by the time the game opens a service
    InstallHooks();

    LightEvent_Init(&g_writerIdle, RESET_STICKY);
    LightEvent_Init(&g_writerResume, RESET_ONESHOT);
    Process::SetProcessEventCallback(OnProcessEvent);
    if (!threadCreate(WriterThread, nullptr, kWriterStack, kWriterPriority, -2, true)) {
        OSD::Notify("UDS shim: could not start the log writer", Color::Red);
    }
}

std::string StatusText() {
    return Utils::Format(
        "Hooks installed: %d\nSessions intercepted: %lu\nActive sessions: %lu\nLog: %s\nBytes written: %lu\n"
        "Dropped lines: %lu",
        g_hookCount, (unsigned long)g_attachCount, (unsigned long)g_activeSessions, g_logPath,
        (unsigned long)g_bytesWritten, (unsigned long)g_droppedLines);
}

} // namespace udsrelay
