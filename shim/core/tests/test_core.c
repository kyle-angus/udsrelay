// PC tests for the shim core. A fake kernel plays both the game (client) and the real
// service, translating static buffers and handles the way the 3DS kernel does.
//
//   make -C shim/core test
//   make -C shim/core fixtures   # also regenerates internal/udstrace/testdata/console_*.log

#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "shim/config.h"
#include "shim/finder.h"
#include "shim/ipc.h"
#include "shim/relay_client.h"
#include "shim/relay_wire.h"
#include "shim/ring.h"
#include "shim/session.h"
#include "shim/sha1.h"
#include "shim/sha256.h"
#include "shim/trace.h"
#include "shim/uds.h"

static int failures;
#define CHECK(cond)                                                                    \
    do {                                                                               \
        if (!(cond)) {                                                                 \
            fprintf(stderr, "%s:%d: CHECK failed: %s\n", __FILE__, __LINE__, #cond);   \
            failures++;                                                                \
        }                                                                              \
    } while (0)

/* ---------------------------------------------------------------- ipc */

static void test_ipc_parse(void) {
    // libctru udsipc_RecvBeaconBroadcastData: 16 normal words, then copy handle + mapped buffer.
    uint32_t w[SHIM_IPC_MAX_WORDS] = {0};
    w[0] = 0x000F0404;
    w[17] = 0x00000000; // IPC_Desc_SharedHandles(1)
    w[18] = 0x1234;
    w[19] = (0x10000u << 4) | 0x8 | 0x4; // IPC_Desc_Buffer(0x10000, IPC_BUFFER_W)
    w[20] = 0x08001000;
    shim_ipc_msg m;
    CHECK(shim_ipc_parse(w, &m) == 0);
    CHECK(m.cmd == 0x000F && m.normal == 16 && m.translate == 4 && m.ndesc == 2);
    CHECK(m.desc[0].kind == SHIM_DESC_COPY_HANDLES && m.desc[0].count == 1 && m.desc[0].word == 17);
    CHECK(m.desc[1].kind == SHIM_DESC_MAPPED_BUFFER && m.desc[1].size == 0x10000 &&
          m.desc[1].id == 2 && m.desc[1].ptr == 0x08001000);

    // SendTo: 6 normal words, static buffer id 5.
    uint32_t s[8] = {0x00170182, 0, 2, 1, 0, 11, 0, 0};
    s[7] = SHIM_IPC_STATIC_DESC(12, 5);
    uint32_t s2[9];
    memcpy(s2, s, sizeof(s));
    s2[8] = 0x08002000;
    CHECK(shim_ipc_parse(s2, &m) == 0);
    CHECK(m.ndesc == 1 && m.desc[0].kind == SHIM_DESC_STATIC_BUFFER && m.desc[0].id == 5 &&
          m.desc[0].size == 12 && m.desc[0].ptr == 0x08002000);

    // EnterExclusiveState: process ID descriptor.
    uint32_t e[4] = {0x00010042, 2, 0x20, 0};
    CHECK(shim_ipc_parse(e, &m) == 0);
    CHECK(m.ndesc == 1 && m.desc[0].kind == SHIM_DESC_PROCESS_ID);

    // Move handles x2 and a translate section that runs past its declared length.
    uint32_t mv[4] = {SHIM_IPC_MAKE_HEADER(1, 0, 3), 0x04000010, 5, 6};
    CHECK(shim_ipc_parse(mv, &m) == 0 && m.desc[0].kind == SHIM_DESC_MOVE_HANDLES && m.desc[0].count == 2);
    uint32_t bad[3] = {SHIM_IPC_MAKE_HEADER(1, 0, 2), 0x04000000, 5}; // 2 handles, 1 word left
    CHECK(shim_ipc_parse(bad, &m) == -1);
}

/* -------------------------------------------------------------- trace */

static void test_trace_format(void) {
    char line[SHIM_TRACE_LINE_MAX];
    shim_time t = {1792000000000000ull, 12000345};
    uint32_t w[4] = {0x00140042, 0, 7, 9}; // 1 normal + 2 translate words
    size_t n = shim_trace_msg(line, sizeof(line), t, "req", "nwm::UDS", 1, 0x14, "PullPacket", 0,
                              "0004000000055D00", w, 0);
    const char *want =
        "[  12.000345] Service <Info> shim:Forward:0: UDSTRACE v=1 kind=req wall=1792000000000000 "
        "port=nwm::UDS sess=1 cmd=0x0014 name=\"PullPacket\" tid=0 proc=\"0004000000055D00\" "
        "hdr=0x00140042 words=0x00140042,0x00000000,0x00000007,0x00000009\n";
    CHECK(n == strlen(want) && strcmp(line, want) == 0);
    if (strcmp(line, want) != 0) {
        fprintf(stderr, "got:  %swant: %s", line, want);
    }

    const uint8_t data[4] = {0xde, 0xad, 0xbe, 0xef};
    n = shim_trace_blob(line, sizeof(line), t, "nwm::UDS", -1, 0x17, "send", "dest=0x0002 chan=1 flags=0x00",
                        data, 4, 2);
    CHECK(strstr(line, "kind=blob wall=1792000000000000 port=nwm::UDS cmd=0x0017 tag=send "
                       "dest=0x0002 chan=1 flags=0x00 size=4 hex=dead\n") != NULL);

    CHECK(shim_trace_msg(line, 40, t, "req", "nwm::UDS", -1, 0x14, "x", 0, "p", w, 0) == 0);

    CHECK(strcmp(shim_trace_tag("nwm::UDS", 0x1D, 0, 1, 0), "netinfo") == 0);
    CHECK(strcmp(shim_trace_tag("nwm::UDS", 0x14, 1, 0, 0), "recv") == 0);
    CHECK(shim_trace_tag("ndm:u", 0x1D, 0, 1, 0) == NULL);
    CHECK(strcmp(shim_command_name("nwm::UDS", 0x1F), "DecryptBeaconData") == 0);
    CHECK(strcmp(shim_command_name("ndm:u", 0x01), "EnterExclusiveState") == 0);
}

/* ------------------------------------------------------------- finder */

static void test_finder(void) {
    const uint32_t base = 0x00100000;
    uint32_t text[64];
    for (int i = 0; i < 64; i++) {
        text[i] = 0xE1A00000; // nop
    }
    // Decoy at 0: loads the constant but never issues svc 0x32 before returning.
    text[0] = 0xE92D4010; // push {r4, lr}
    text[1] = 0xE59F0000 | (4 * (40 - 1 - 2)); // ldr r0, [pc, #...] -> text[40]
    text[2] = 0xE8BD8010; // pop {r4, pc}
    // Real function at 8: push, mrc, ldr literal, conditional early return, svc 0x32, pop.
    text[8] = 0xE92D41F0;  // push {r4-r8, lr}
    text[9] = 0xEE1D4F70;  // mrc p15, 0, r4, c13, c0, 3
    text[10] = 0xE59FC000 | (4 * (41 - 10 - 2)); // ldr r12, [pc, #...] -> text[41]
    text[11] = 0x18BD81F0; // popne {r4-r8, pc}  (early exit must not end the search)
    text[12] = 0xEF000032; // svc 0x32
    text[13] = 0xE8BD81F0; // pop {r4-r8, pc}
    text[40] = SHIM_SRV_GET_SERVICE_HANDLE_HDR;
    text[41] = SHIM_SRV_GET_SERVICE_HANDLE_HDR;
    // Unrelated literal-pool load of another constant.
    text[20] = 0xE92D4000;
    text[21] = 0xE59F0000 | (4 * (42 - 21 - 2));
    text[22] = 0xEF000032;
    text[42] = 0x00040100;

    shim_hook_candidate c[4];
    size_t n = shim_find_get_service_handle(text, 64, base, c, 4);
    CHECK(n == 1);
    CHECK(n >= 1 && c[0].func == base + 8 * 4 && c[0].literal == base + 41 * 4 && c[0].svc == base + 12 * 4);
}

static uint32_t mem_words[4] = {0x1111, 0x00022222, 0x00022222, 0x3333};
static int fake_read(void *ctx, uint32_t addr, uint32_t *value) {
    (void)ctx;
    if (addr < 0x1000 || addr >= 0x1010) {
        return -1;
    }
    *value = mem_words[(addr - 0x1000) / 4];
    return 0;
}

static void test_pick_out_arg(void) {
    uint32_t args[4] = {0x1000, 0x2000, 3, 0};
    CHECK(shim_pick_out_arg(args, 0x1111, fake_read, NULL) == 0);
    uint32_t args2[4] = {7, 0x1004, 0x1004, 0x1002};
    CHECK(shim_pick_out_arg(args2, 0x00022222, fake_read, NULL) == 1); // same pointer twice is fine
    uint32_t args3[4] = {0x1004, 0x1008, 0, 0};
    CHECK(shim_pick_out_arg(args3, 0x00022222, fake_read, NULL) == -1); // two distinct pointers
    uint32_t args4[4] = {0x1000, 0, 0, 0};
    CHECK(shim_pick_out_arg(args4, 0x9999, fake_read, NULL) == -1);
}

/* --------------------------------------------------- config and ring */

static void test_config(void) {
    shim_config cfg;
    shim_config_defaults(&cfg);
    CHECK(cfg.nports == 2 && shim_config_traces(&cfg, "nwm::UDS") && shim_config_traces(&cfg, "ndm:u"));
    CHECK(!shim_config_traces(&cfg, "ac:u") && cfg.payload_max == 512);
    const char ini[] = "# shim settings\n ports = nwm::UDS, ndm:u , ac:u\npayload_max=-1\nbogus\nfoo=bar\npayload_max = x\n";
    CHECK(shim_config_parse(&cfg, ini, sizeof(ini) - 1) == 3);
    CHECK(cfg.nports == 3 && shim_config_traces(&cfg, "ac:u") && cfg.payload_max == -1);
    const char toolong[] = "ports=nwm::UDSX,ndm:u";
    shim_config_parse(&cfg, toolong, sizeof(toolong) - 1);
    CHECK(cfg.nports == 1 && shim_config_traces(&cfg, "ndm:u"));

    CHECK(cfg.relay_host[0] == '\0' && cfg.lobby[0] == '\0' && !cfg.fake_ndm);
    const char relay[] = "relay = relay.example.net:28300\nlobby = 3q2-hEXAMPLE key\nfake_ndm = 1\n";
    CHECK(shim_config_parse(&cfg, relay, sizeof(relay) - 1) == 0);
    CHECK(strcmp(cfg.relay_host, "relay.example.net") == 0 && cfg.relay_port == 28300);
    CHECK(strcmp(cfg.lobby, "3q2-hEXAMPLE key") == 0 && cfg.fake_ndm == 1);
    const char badrelay[] = "relay = nohost\nrelay = :1\nrelay = h:99999\nrelay = h:12x\nfake_ndm = yes\n";
    CHECK(shim_config_parse(&cfg, badrelay, sizeof(badrelay) - 1) == 5);
    CHECK(strcmp(cfg.relay_host, "relay.example.net") == 0 && cfg.relay_port == 28300); // unchanged

    CHECK(!cfg.relay_mode);
    const char mode[] = "mode = relay\n";
    CHECK(shim_config_parse(&cfg, mode, sizeof(mode) - 1) == 0 && cfg.relay_mode == 1);
    const char modes[] = "mode = Relay\nmode = passthrough\n";
    CHECK(shim_config_parse(&cfg, modes, sizeof(modes) - 1) == 1 && cfg.relay_mode == 0);
}

static void test_ring(void) {
    char mem[16], out[32];
    shim_ring r;
    shim_ring_init(&r, mem, sizeof(mem));
    CHECK(shim_ring_push(&r, "0123456789", 10) == 0);
    CHECK(shim_ring_push(&r, "abcdefgh", 8) == -1 && r.dropped_lines == 1);
    CHECK(shim_ring_pop(&r, out, 4) == 4 && memcmp(out, "0123", 4) == 0);
    CHECK(shim_ring_push(&r, "abcdefgh", 8) == 0); // wraps
    size_t n = shim_ring_pop(&r, out, sizeof(out));
    CHECK(n == 14 && memcmp(out, "456789abcdefgh", 14) == 0 && r.used == 0);
}

/* ------------------------------------------------ fake kernel session */

#define ARENA_BASE 0x08000000u
#define ARENA_SIZE (1u << 20)

typedef struct {
    uint32_t words[SHIM_IPC_MAX_WORDS];
    // Static buffer payloads and mapped buffer contents for this message.
    const uint8_t *sbuf[SHIM_STATIC_SLOTS];
    uint32_t sbuf_len[SHIM_STATIC_SLOTS];
} fake_msg;

typedef struct fake_kernel fake_kernel;
typedef void (*service_fn)(fake_kernel *k, const uint32_t *req, fake_msg *reply);

struct fake_kernel {
    uint8_t *arena;
    size_t arena_used;
    uint32_t *cmdbuf;
    uint32_t *static_bufs;
    // Game side.
    fake_msg *requests;
    int nrequests, next_request;
    uint32_t delivered[16][SHIM_IPC_MAX_WORDS];
    uint32_t replies[16][SHIM_IPC_MAX_WORDS];
    uint8_t reply_data[16][64];
    int nreplies;
    // Service side.
    service_fn service;
    uint32_t forwarded[16][SHIM_IPC_MAX_WORDS];
    int nforwarded;
    uint8_t *game_mapped; // the game's mapped buffer, shared by the scan request
    // Handles.
    uint32_t next_handle;
    int open[512];
    int double_close;
    // Log.
    FILE *log;
    char text[1 << 16];
    size_t text_len;
    uint64_t clock;
};

static void *arena_alloc(fake_kernel *k, size_t n) {
    void *p = k->arena + k->arena_used;
    k->arena_used += (n + 15) & ~(size_t)15;
    if (k->arena_used > ARENA_SIZE) {
        abort();
    }
    return p;
}

static uint32_t k_addr(fake_kernel *k, const void *p) {
    return ARENA_BASE + (uint32_t)((const uint8_t *)p - k->arena);
}

static uint8_t *k_ptr(fake_kernel *k, uint32_t addr) {
    if (addr < ARENA_BASE || addr >= ARENA_BASE + ARENA_SIZE) {
        return NULL;
    }
    return k->arena + (addr - ARENA_BASE);
}

static uint32_t dup_handle(fake_kernel *k) {
    uint32_t h = k->next_handle++;
    k->open[h] = 1;
    return h;
}

// Kernel delivery: copies `src` into the receiver's cmdbuf, translating static buffers into the
// receiver's current static buffer descriptors and duplicating copy handles.
static void deliver(fake_kernel *k, const fake_msg *src) {
    memcpy(k->cmdbuf, src->words, shim_ipc_words(src->words[0]) * 4);
    shim_ipc_msg m;
    if (shim_ipc_parse(src->words, &m) != 0) {
        abort();
    }
    for (unsigned i = 0; i < m.ndesc; i++) {
        shim_desc *d = &m.desc[i];
        if (d->kind == SHIM_DESC_COPY_HANDLES) {
            for (unsigned c = 0; c < d->count; c++) {
                k->cmdbuf[d->word + 1 + c] = dup_handle(k);
            }
        } else if (d->kind == SHIM_DESC_STATIC_BUFFER) {
            const uint32_t rdesc = k->static_bufs[2 * d->id];
            CHECK((rdesc >> 14) >= src->sbuf_len[d->id]);
            uint8_t *dst = k_ptr(k, k->static_bufs[2 * d->id + 1]);
            memcpy(dst, src->sbuf[d->id], src->sbuf_len[d->id]);
            k->cmdbuf[d->word + 1] = k->static_bufs[2 * d->id + 1];
        }
    }
}

static uint32_t *f_cmdbuf(void *ctx) { return ((fake_kernel *)ctx)->cmdbuf; }
static uint32_t *f_static(void *ctx) { return ((fake_kernel *)ctx)->static_bufs; }

static shim_result_t f_reply_and_receive(void *ctx, int32_t *index, const shim_handle_t *handles,
                                         int32_t count, shim_handle_t reply_target) {
    fake_kernel *k = ctx;
    CHECK(count == 1 && handles[0] == 1);
    if (reply_target) {
        CHECK(reply_target == 1);
        memcpy(k->replies[k->nreplies], k->cmdbuf, shim_ipc_words(k->cmdbuf[0]) * 4);
        // Kernel copies reply static buffers to the game's buffers.
        shim_ipc_msg m;
        if (shim_ipc_parse(k->cmdbuf, &m) == 0) {
            for (unsigned i = 0; i < m.ndesc; i++) {
                if (m.desc[i].kind == SHIM_DESC_STATIC_BUFFER) {
                    const uint8_t *p = k_ptr(k, m.desc[i].ptr);
                    CHECK(p != NULL);
                    if (p) {
                        memcpy(k->reply_data[k->nreplies], p, m.desc[i].size < 64 ? m.desc[i].size : 64);
                    }
                }
            }
        }
        k->nreplies++;
    }
    if (k->next_request == k->nrequests) {
        return SHIM_ERR_SESSION_CLOSED;
    }
    // The receive buffers must be the request bank while waiting for the game.
    fake_msg *req = &k->requests[k->next_request];
    deliver(k, req);
    memcpy(k->delivered[k->next_request], k->cmdbuf, sizeof(k->delivered[0]));
    k->next_request++;
    *index = 0;
    return 0;
}

static shim_result_t f_send_sync(void *ctx, shim_handle_t session) {
    fake_kernel *k = ctx;
    CHECK(session == 2);
    memcpy(k->forwarded[k->nforwarded++], k->cmdbuf, shim_ipc_words(k->cmdbuf[0]) * 4);
    fake_msg reply;
    memset(&reply, 0, sizeof(reply));
    k->service(k, k->cmdbuf, &reply);
    deliver(k, &reply);
    return 0;
}

static void f_close(void *ctx, shim_handle_t h) {
    fake_kernel *k = ctx;
    if (h == 1 || h == 2) {
        return; // the session handles
    }
    if (h >= 512 || !k->open[h]) {
        k->double_close++;
        return;
    }
    k->open[h] = 0;
}

static uint32_t f_addr_of(void *ctx, const void *p) { return k_addr(ctx, p); }
static const uint8_t *f_view(void *ctx, uint32_t addr, uint32_t size) {
    (void)size;
    return k_ptr(ctx, addr);
}
static shim_time f_now(void *ctx) {
    fake_kernel *k = ctx;
    k->clock += 250;
    shim_time t = {1792000000000000ull + k->clock, k->clock};
    return t;
}
static void f_log(void *ctx, const char *line, size_t len) {
    fake_kernel *k = ctx;
    if (k->text_len + len < sizeof(k->text)) {
        memcpy(k->text + k->text_len, line, len);
        k->text_len += len;
        k->text[k->text_len] = '\0';
    }
    if (k->log) {
        fwrite(line, 1, len, k->log);
    }
}

static const uint8_t NETINFO_COMM_ID[4] = {0x00, 0x12, 0x34, 0x56};
static uint8_t netinfo[0x108];
static const uint8_t PASSPHRASE[] = "FIXTURE-PASSPHRASE-NOT-REAL";
static const uint8_t SEND_DATA[] = "trade-offer";
static const uint8_t RECV_DATA[] = "trade-ack!!";

// A host-side nwm::UDS: replies the way real NWM does for each command used below.
static void fake_nwm(fake_kernel *k, const uint32_t *req, fake_msg *reply) {
    uint32_t *w = reply->words;
    switch (SHIM_IPC_CMD(req[0])) {
    case 0x001B: // InitializeWithVersion -> copy handle (connection status event)
        w[0] = SHIM_IPC_MAKE_HEADER(0x1B, 1, 2);
        w[1] = 0;
        w[2] = 0;
        w[3] = 0x77; // the service's own handle; the kernel duplicates it for us
        break;
    case 0x001D: // BeginHostingNetwork
        w[0] = SHIM_IPC_MAKE_HEADER(0x1D, 1, 0);
        break;
    case 0x0017: // SendTo
        w[0] = SHIM_IPC_MAKE_HEADER(0x17, 1, 0);
        break;
    case 0x0014: // PullPacket -> size, src node, static buffer id 0
        w[0] = SHIM_IPC_MAKE_HEADER(0x14, 3, 2);
        w[2] = sizeof(RECV_DATA) - 1;
        w[3] = 2;
        w[4] = SHIM_IPC_STATIC_DESC(sizeof(RECV_DATA) - 1, 0);
        w[5] = 0;
        reply->sbuf[0] = RECV_DATA;
        reply->sbuf_len[0] = sizeof(RECV_DATA) - 1;
        break;
    case 0x000F: { // RecvBeaconBroadcastData -> fill the mapped buffer, return it
        uint32_t header[3] = {0x100, 0x20, 1};
        memcpy(k->game_mapped, header, sizeof(header));
        memset(k->game_mapped + 12, 0xAB, 0x20 - 12);
        w[0] = SHIM_IPC_MAKE_HEADER(0xF, 1, 2);
        w[2] = req[19];
        w[3] = req[20];
        break;
    }
    default:
        w[0] = SHIM_IPC_MAKE_HEADER(SHIM_IPC_CMD(req[0]), 1, 0);
        w[1] = 0xC8A0CFEF; // arbitrary failure for unexpected commands
    }
}

static void test_session(const char *fixture_path) {
    fake_kernel k;
    memset(&k, 0, sizeof(k));
    k.arena = calloc(1, ARENA_SIZE);
    k.cmdbuf = arena_alloc(&k, 0x100);
    k.static_bufs = arena_alloc(&k, 0x80);
    k.service = fake_nwm;
    k.next_handle = 0x100;
    k.game_mapped = arena_alloc(&k, 0x100);
    if (fixture_path) {
        k.log = fopen(fixture_path, "w");
    }

    memset(netinfo, 0, sizeof(netinfo));
    memcpy(netinfo + 0x10, NETINFO_COMM_ID, 4);
    netinfo[0x14] = 0x02;
    netinfo[0x1D] = 2;

    fake_msg reqs[5];
    memset(reqs, 0, sizeof(reqs));
    // InitializeWithVersion: 12 normal words, shared memory copy handle.
    reqs[0].words[0] = 0x001B0302;
    reqs[0].words[1] = 0x3000;
    reqs[0].words[12] = 0x400;
    reqs[0].words[13] = 0; // IPC_Desc_SharedHandles(1)
    reqs[0].words[14] = 0x55;
    // BeginHostingNetwork: netinfo (id 1) and passphrase (id 0).
    reqs[1].words[0] = 0x001D0044;
    reqs[1].words[1] = sizeof(PASSPHRASE) - 1;
    reqs[1].words[2] = SHIM_IPC_STATIC_DESC(sizeof(netinfo), 1);
    reqs[1].words[4] = SHIM_IPC_STATIC_DESC(sizeof(PASSPHRASE) - 1, 0);
    reqs[1].sbuf[1] = netinfo;
    reqs[1].sbuf_len[1] = sizeof(netinfo);
    reqs[1].sbuf[0] = PASSPHRASE;
    reqs[1].sbuf_len[0] = sizeof(PASSPHRASE) - 1;
    // SendTo node 2, channel 1, 11 bytes (aligned 12).
    reqs[2].words[0] = 0x00170182;
    reqs[2].words[2] = 2;
    reqs[2].words[3] = 1;
    reqs[2].words[5] = sizeof(SEND_DATA) - 1;
    reqs[2].words[7] = SHIM_IPC_STATIC_DESC(12, 5);
    reqs[2].sbuf[5] = SEND_DATA;
    reqs[2].sbuf_len[5] = sizeof(SEND_DATA) - 1;
    // PullPacket on bind 1.
    reqs[3].words[0] = 0x001400C0;
    reqs[3].words[1] = 1;
    reqs[3].words[2] = 0x5F4;
    reqs[3].words[3] = 0x5F4;
    // RecvBeaconBroadcastData: event copy handle + mapped output buffer.
    reqs[4].words[0] = 0x000F0404;
    reqs[4].words[1] = 0x100;
    reqs[4].words[15] = 0x00123456;
    reqs[4].words[16] = 2;
    reqs[4].words[17] = 0;
    reqs[4].words[18] = 0x66;
    reqs[4].words[19] = (0x100u << 4) | 0x8 | 0x4;
    reqs[4].words[20] = k_addr(&k, k.game_mapped);
    k.requests = reqs;
    k.nrequests = 5;

    shim_session s;
    memset(&s, 0, sizeof(s));
    s.port = "nwm::UDS";
    s.server = 1;
    s.real = 2;
    s.proc = "000400000FFFFF00";
    s.payload_max = 512;
    s.slot_size = 0x200;
    s.bank_req = arena_alloc(&k, SHIM_STATIC_SLOTS * s.slot_size);
    s.bank_rsp = arena_alloc(&k, SHIM_STATIC_SLOTS * s.slot_size);
    s.line = malloc(SHIM_TRACE_LINE_MAX);

    shim_ops ops = {&k, f_cmdbuf, f_static, f_send_sync, f_reply_and_receive, f_close,
                    f_addr_of, f_view, f_now, f_log, NULL};
    CHECK(shim_session_run(&s, &ops) == 0);

    CHECK(s.requests == 5 && s.failures == 0);
    CHECK(k.nforwarded == 5 && k.nreplies == 5);
    // Forwarded requests are exactly what the kernel delivered to us.
    for (int i = 0; i < 5; i++) {
        CHECK(memcmp(k.forwarded[i], k.delivered[i], shim_ipc_words(k.delivered[i][0]) * 4) == 0);
    }
    // Static buffers in the forwarded BeginHostingNetwork point into the request bank.
    CHECK(k.forwarded[1][3] == k_addr(&k, s.bank_req + 1 * s.slot_size));
    CHECK(k.forwarded[1][5] == k_addr(&k, s.bank_req + 0 * s.slot_size));
    // PullPacket's reply data arrived in the reply bank and reached the game.
    CHECK(k.replies[3][5] == k_addr(&k, s.bank_rsp + 0 * s.slot_size));
    CHECK(memcmp(k.reply_data[3], RECV_DATA, sizeof(RECV_DATA) - 1) == 0);
    CHECK(k.replies[3][2] == sizeof(RECV_DATA) - 1 && k.replies[3][3] == 2);
    // Every handle duplicate the shim received was closed exactly once.
    int still_open = 0;
    for (int h = 0x100; h < (int)k.next_handle; h++) {
        still_open += k.open[h];
    }
    CHECK(still_open == 0 && k.double_close == 0);

    // Log content.
    CHECK(strstr(k.text, "kind=req wall=") && strstr(k.text, "name=\"InitializeWithVersion\""));
    CHECK(strstr(k.text, "tag=netinfo size=264 hex=000000000000000000000000000000000012345602"));
    CHECK(strstr(k.text, "tag=passphrase size=27 hex=464958"));
    CHECK(strstr(k.text, "tag=send dest=0x0002 chan=1 flags=0x00 size=11 hex=74726164652d6f66666572\n"));
    CHECK(strstr(k.text, "tag=recv bind=1 src=0x0002 size=11 hex=74726164652d61636b2121\n"));
    CHECK(strstr(k.text, "tag=beacons size=32 hex=00010000200000000100000"));
    CHECK(strstr(k.text, "kind=rsp") && strstr(k.text, "deferred=0"));

    if (k.log) {
        fclose(k.log);
    }
    free(s.line);
    free(k.arena);
}

// A failing real service: the game must get the error as the reply result.
static void failing_nwm(fake_kernel *k, const uint32_t *req, fake_msg *reply) {
    (void)k;
    (void)req;
    reply->words[0] = SHIM_IPC_MAKE_HEADER(0xB, 1, 0);
    reply->words[1] = 0xC8A0CFEF;
}

static void test_session_error_result(void) {
    fake_kernel k;
    memset(&k, 0, sizeof(k));
    k.arena = calloc(1, ARENA_SIZE);
    k.cmdbuf = arena_alloc(&k, 0x100);
    k.static_bufs = arena_alloc(&k, 0x80);
    k.service = failing_nwm;
    k.next_handle = 0x100;
    fake_msg req;
    memset(&req, 0, sizeof(req));
    req.words[0] = 0x000B0000;
    k.requests = &req;
    k.nrequests = 1;
    shim_session s;
    memset(&s, 0, sizeof(s));
    s.port = "nwm::UDS";
    s.server = 1;
    s.real = 2;
    s.proc = "";
    s.slot_size = 0x100;
    s.bank_req = arena_alloc(&k, SHIM_STATIC_SLOTS * s.slot_size);
    s.bank_rsp = arena_alloc(&k, SHIM_STATIC_SLOTS * s.slot_size);
    s.line = malloc(SHIM_TRACE_LINE_MAX);
    shim_ops ops = {&k, f_cmdbuf, f_static, f_send_sync, f_reply_and_receive, f_close,
                    f_addr_of, f_view, f_now, f_log, NULL};
    CHECK(shim_session_run(&s, &ops) == 0);
    CHECK(k.nreplies == 1 && k.replies[0][1] == 0xC8A0CFEF);
    CHECK(strstr(k.text, "words=0x000b0040,0xc8a0cfef\n") != NULL);
    free(s.line);
    free(k.arena);
}

// ops->answer replies to ndm:u's EnterExclusiveState itself: the real service never runs. It
// also answers 0x0003 with a handle of its own, which the session must not close.
static int answer_ndm(void *ctx, shim_session *s, uint32_t *cmd) {
    (void)ctx;
    if (strcmp(s->port, "ndm:u") != 0) {
        return 0;
    }
    switch (cmd[0] >> 16) {
    case 0x0001:
        cmd[0] = SHIM_IPC_MAKE_HEADER(0x0001, 1, 0);
        cmd[1] = 0;
        return 1;
    case 0x0003:
        cmd[0] = SHIM_IPC_MAKE_HEADER(0x0003, 1, 2);
        cmd[1] = 0;
        cmd[2] = 0;    // copy one handle
        cmd[3] = 0x77; // the answerer's; never opened in the fake kernel, so closing it shows up
        return 1;
    }
    return 0;
}

static void test_session_answer(void) {
    fake_kernel k;
    memset(&k, 0, sizeof(k));
    k.arena = calloc(1, ARENA_SIZE);
    k.cmdbuf = arena_alloc(&k, 0x100);
    k.static_bufs = arena_alloc(&k, 0x80);
    k.service = failing_nwm; // would answer 0xC8A0CFEF if it were reached
    k.next_handle = 0x100;
    fake_msg req[4];
    memset(req, 0, sizeof(req));
    req[0].words[0] = 0x00010042; // EnterExclusiveState(2) + process ID descriptor
    req[0].words[1] = 2;
    req[0].words[2] = 0x20;
    req[1].words[0] = 0x00020002; // LeaveExclusiveState: forwarded
    req[1].words[1] = 0x20;
    req[2].words[0] = 0x00030000; // answered with a handle
    req[3] = req[0];              // answered again: no second note
    k.requests = req;
    k.nrequests = 4;
    shim_session s;
    memset(&s, 0, sizeof(s));
    s.port = "ndm:u";
    s.server = 1;
    s.real = 2;
    s.proc = "";
    s.slot_size = 0x100;
    s.bank_req = arena_alloc(&k, SHIM_STATIC_SLOTS * s.slot_size);
    s.bank_rsp = arena_alloc(&k, SHIM_STATIC_SLOTS * s.slot_size);
    s.line = malloc(SHIM_TRACE_LINE_MAX);
    shim_ops ops = {&k, f_cmdbuf, f_static, f_send_sync, f_reply_and_receive, f_close,
                    f_addr_of, f_view, f_now, f_log, answer_ndm};
    CHECK(shim_session_run(&s, &ops) == 0);
    CHECK(k.nreplies == 4 && k.replies[0][0] == 0x00010040 && k.replies[0][1] == 0);
    CHECK(k.replies[1][1] == 0xC8A0CFEF); // the second request did reach the service
    CHECK(s.answered == 3 && s.requests == 4);
    CHECK(k.replies[2][3] == 0x77 && k.double_close == 0);
    const char *first = strstr(k.text, "ndm:u: answered 0x0001 locally");
    CHECK(first != NULL && strstr(first + strlen("ndm:u: answered"), "answered 0x0001 locally") == NULL);
    free(s.line);
    free(k.arena);
}

/* -------------------------------------------------------------- relay wire */

static int hex_equal(const uint8_t *b, size_t n, const char *hex) {
    if (strlen(hex) != 2 * n) {
        return 0;
    }
    for (size_t i = 0; i < n; i++) {
        unsigned v;
        if (sscanf(hex + 2 * i, "%2x", &v) != 1 || v != b[i]) {
            return 0;
        }
    }
    return 1;
}

// Published vectors: FIPS 180-2 ("abc", one million 'a') and RFC 4231 cases 2 and 6.
static void test_sha256(void) {
    uint8_t out[SHIM_SHA256_SIZE];
    shim_sha256("abc", 3, out);
    CHECK(hex_equal(out, 32, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"));

    shim_sha256_ctx s;
    shim_sha256_init(&s);
    char a[1001];
    memset(a, 'a', sizeof(a));
    for (int i = 0; i < 1000; i++) {
        shim_sha256_update(&s, a, i % 2 ? 999 : 1001); // odd chunk sizes cross block boundaries
    }
    shim_sha256_final(&s, out);
    CHECK(hex_equal(out, 32, "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0"));

    shim_hmac_sha256("Jefe", 4, "what do ya want for nothing?", 28, out);
    CHECK(hex_equal(out, 32, "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"));

    uint8_t longkey[131];
    memset(longkey, 0xaa, sizeof(longkey));
    const char *msg = "Test Using Larger Than Block-Size Key - Hash Key First";
    shim_hmac_sha256(longkey, sizeof(longkey), msg, strlen(msg), out);
    CHECK(hex_equal(out, 32, "60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54"));
}

// The same vectors are checked on the Go side (internal/relay/vectors_test.go); they were
// computed independently with Python's hashlib and hmac.
static void test_relay_wire(void) {
    const char *key = "test-key";
    CHECK(shim_relay_lobby_id(key, 8) == 0x6ab979dd);

    uint8_t buf[SHIM_RELAY_MAX_DATAGRAM];
    size_t n = shim_relay_hello(key, 8, 0x01020304, 7, buf, sizeof(buf));
    CHECK(n == 32 && hex_equal(buf, n, "5552020101020304000700006ab979dd3cc2c53977b4c5f9fb54fe29371441f3"));
    CHECK(shim_relay_hello(key, 8, 1, 1, buf, 31) == 0); // too small for the tag

    // DATA from node 1 to node 2 on channel 243, flags 2.
    const shim_relay_header dh = {SHIM_RELAY_DATA, 0x01020304, 9, 0};
    shim_relay_header_put(buf, &dh);
    const uint8_t data[] = {0, 1, 0, 2, 243, 2, 2, 1, 2, 1};
    memcpy(buf + SHIM_RELAY_HEADER_SIZE, data, sizeof(data));
    n = shim_relay_seal(key, 8, buf, SHIM_RELAY_HEADER_SIZE + sizeof(data), sizeof(buf));
    CHECK(hex_equal(buf, n, "5552020c010203040009000000010002f3020201020103570ab1ecbb5ebb3e12ef9d770af768"));

    CHECK(shim_relay_open(key, 8, buf, n) == (long)(n - SHIM_RELAY_TAG_SIZE));
    CHECK(shim_relay_open("wrong-key", 9, buf, n) == -1);
    buf[14] ^= 1; // a flipped payload bit
    CHECK(shim_relay_open(key, 8, buf, n) == -1);
    CHECK(shim_relay_open(key, 8, buf, SHIM_RELAY_HEADER_SIZE + SHIM_RELAY_TAG_SIZE - 1) == -1);

    shim_relay_header h;
    CHECK(shim_relay_header_get(buf, n, &h) == 0 && h.type == SHIM_RELAY_DATA && h.session == 0x01020304 &&
          h.seq == 9 && h.ack == 0);
    buf[2] = 1; // version 1
    CHECK(shim_relay_header_get(buf, n, &h) == -1);
}

/* -------------------------------------------------------------- relay client */

// A fake transport: records what the client sends and what it reports.
typedef struct {
    uint8_t sent[16][SHIM_RELAY_MAX_DATAGRAM];
    size_t sent_len[16];
    int nsent;
    int data, events, resets, scans;
    shim_relay_event last_event;
    uint16_t data_src;
    uint8_t data_payload[16];
} fake_net;

static void fn_send(void *ctx, const uint8_t *b, size_t n) {
    fake_net *f = (fake_net *)ctx;
    if (f->nsent < 16) {
        memcpy(f->sent[f->nsent], b, n);
        f->sent_len[f->nsent] = n;
    }
    f->nsent++;
}
static void fn_data(void *ctx, uint16_t src, uint16_t dst, uint8_t ch, uint8_t fl, const uint8_t *p, size_t n) {
    fake_net *f = (fake_net *)ctx;
    (void)dst, (void)ch, (void)fl;
    f->data++;
    f->data_src = src;
    memcpy(f->data_payload, p, n < 16 ? n : 16);
}
static void fn_event(void *ctx, const shim_relay_event *e) {
    fake_net *f = (fake_net *)ctx;
    f->events++;
    f->last_event = *e;
}
static void fn_reset(void *ctx) { ((fake_net *)ctx)->resets++; }
static void fn_scan(void *ctx, uint8_t index, uint8_t total, const uint8_t *net, size_t n) {
    (void)index, (void)total, (void)net, (void)n;
    ((fake_net *)ctx)->scans++;
}

// Feeds the client a datagram as the relay would send it.
static void from_relay(shim_relay_client *c, const char *key, uint8_t type, uint16_t seq, uint16_t ack,
                       const uint8_t *p, size_t n, uint64_t now) {
    uint8_t b[SHIM_RELAY_MAX_DATAGRAM];
    const shim_relay_header h = {type, c->session, seq, ack};
    shim_relay_header_put(b, &h);
    memcpy(b + SHIM_RELAY_HEADER_SIZE, p, n);
    const size_t len = shim_relay_seal(key, strlen(key), b, SHIM_RELAY_HEADER_SIZE + n, sizeof(b));
    shim_relay_input(c, b, len, now);
}

static shim_relay_header sent_header(const fake_net *f, int i) {
    shim_relay_header h = {0, 0, 0, 0};
    shim_relay_header_get(f->sent[i], f->sent_len[i], &h);
    return h;
}

static void test_relay_client(void) {
    const char *key = "test-key";
    fake_net f;
    memset(&f, 0, sizeof(f));
    const shim_relay_ops ops = {&f, fn_send, fn_data, fn_event, fn_reset, fn_scan};
    shim_relay_client c;
    CHECK(shim_relay_client_init(&c, &ops, key, 0, 1) == -1); // empty key
    CHECK(shim_relay_client_init(&c, &ops, key, 8, 0x01020304) == 0);

    // HELLO goes out sealed and names the lobby; HELLO_ACK sets the keepalive interval.
    CHECK(shim_relay_hello_request(&c, 1000) == 0);
    CHECK(f.nsent == 1 && shim_relay_open(key, 8, f.sent[0], f.sent_len[0]) == 16);
    CHECK(sent_header(&f, 0).type == SHIM_RELAY_HELLO);
    CHECK(shim_relay_hello_request(&c, 1000) == -1); // one request at a time
    shim_relay_poll(&c, 1249);
    CHECK(f.nsent == 1);
    shim_relay_poll(&c, 1250); // retry
    CHECK(f.nsent == 2 && memcmp(f.sent[0], f.sent[1], f.sent_len[0]) == 0);
    const uint8_t hello_ack[] = {0, 0x01, 0xF4, 0x13, 0x88}; // OK, keepalive 500 ms, timeout 5 s
    from_relay(&c, key, SHIM_RELAY_HELLO_ACK, 1, sent_header(&f, 0).seq, hello_ack, sizeof(hello_ack), 1300);
    CHECK(c.req_state == SHIM_RELAY_REQ_DONE && c.status == 0 && c.hello_ok && c.keepalive_ms == 500);
    shim_relay_request_clear(&c);

    // A keepalive every 500 ms, nothing in between.
    f.nsent = 0;
    shim_relay_poll(&c, 1500);
    CHECK(f.nsent == 1 && sent_header(&f, 0).type == SHIM_RELAY_KEEPALIVE);
    shim_relay_poll(&c, 1999);
    CHECK(f.nsent == 1);

    // A request that gets no answer fails after 3 s.
    uint8_t p[512];
    size_t n = shim_relay_eject(p, sizeof(p), 2);
    CHECK(shim_relay_request(&c, SHIM_RELAY_EJECT, p, n, 2000) == 0);
    shim_relay_poll(&c, 4999);
    CHECK(c.req_state == SHIM_RELAY_REQ_PENDING);
    shim_relay_poll(&c, 5000);
    CHECK(c.req_state == SHIM_RELAY_REQ_FAILED);
    shim_relay_request_clear(&c);

    // A scan collects one datagram per network, ignoring duplicates.
    f.nsent = 0;
    n = shim_relay_scan(p, sizeof(p), 0x00055d10, 0x01);
    CHECK(shim_relay_request(&c, SHIM_RELAY_SCAN_REQ, p, n, 6000) == 0);
    const uint16_t scan_seq = sent_header(&f, 0).seq;
    uint8_t net[3 + SHIM_RELAY_NETINFO_SIZE + 2] = {0, 0, 2};
    from_relay(&c, key, SHIM_RELAY_SCAN_RESP, 2, scan_seq, net, sizeof(net), 6010);
    from_relay(&c, key, SHIM_RELAY_SCAN_RESP, 3, scan_seq, net, sizeof(net), 6010); // duplicate index 0
    CHECK(f.scans == 1 && c.req_state == SHIM_RELAY_REQ_PENDING);
    net[1] = 1;
    from_relay(&c, key, SHIM_RELAY_SCAN_RESP, 4, scan_seq, net, sizeof(net), 6020);
    CHECK(f.scans == 2 && c.req_state == SHIM_RELAY_REQ_DONE && c.status == 0);
    shim_relay_request_clear(&c);
    // ...and returns what it has if one network's datagram is lost.
    CHECK(shim_relay_request(&c, SHIM_RELAY_SCAN_REQ, p, n, 7000) == 0);
    net[1] = 0;
    from_relay(&c, key, SHIM_RELAY_SCAN_RESP, 5, c.req_seq, net, sizeof(net), 7010);
    shim_relay_poll(&c, 10000);
    CHECK(c.req_state == SHIM_RELAY_REQ_DONE && f.scans == 3);
    shim_relay_request_clear(&c);
    // A response for another seq is ignored.
    n = shim_relay_eject(p, sizeof(p), 2);
    CHECK(shim_relay_request(&c, SHIM_RELAY_EJECT, p, n, 11000) == 0);
    const uint8_t ok[] = {0};
    from_relay(&c, key, SHIM_RELAY_EJECT_ACK, 6, (uint16_t)(c.req_seq - 1), ok, 1, 11010);
    CHECK(c.req_state == SHIM_RELAY_REQ_PENDING);
    from_relay(&c, key, SHIM_RELAY_EJECT_ACK, 7, c.req_seq, ok, 1, 11020);
    CHECK(c.req_state == SHIM_RELAY_REQ_DONE);
    shim_relay_request_clear(&c);

    // NODE_EVENT: always ACKed, delivered once.
    f.nsent = 0;
    uint8_t ev[11 + SHIM_RELAY_NODEINFO_SIZE] = {SHIM_RELAY_EV_JOINED, 0, 0xAA, 0xBB, 0xCC, 0xDD, 0, 2, 0, 3, 2};
    ev[11] = 'k';
    from_relay(&c, key, SHIM_RELAY_NODE_EVENT, 40, 0, ev, sizeof(ev), 12000);
    from_relay(&c, key, SHIM_RELAY_NODE_EVENT, 40, 0, ev, sizeof(ev), 12200); // retransmission
    CHECK(f.events == 1 && f.last_event.node == 2 && f.last_event.bitmask == 3 &&
          f.last_event.network_id == 0xAABBCCDD && f.last_event.info[0] == 'k');
    CHECK(f.nsent == 2 && sent_header(&f, 0).type == SHIM_RELAY_ACK && sent_header(&f, 0).ack == 40 &&
          sent_header(&f, 1).ack == 40);
    ev[0] = SHIM_RELAY_EV_LEFT;
    from_relay(&c, key, SHIM_RELAY_NODE_EVENT, 41, 0, ev, sizeof(ev), 12300);
    CHECK(f.events == 2 && f.last_event.kind == SHIM_RELAY_EV_LEFT);

    // DATA both ways.
    const uint8_t in[] = {0, 2, 0, 1, 243, 2, 'h', 'i'};
    from_relay(&c, key, SHIM_RELAY_DATA, 50, 0, in, sizeof(in), 12400);
    CHECK(f.data == 1 && f.data_src == 2 && memcmp(f.data_payload, "hi", 2) == 0);
    f.nsent = 0;
    CHECK(shim_relay_send_data(&c, 2, 243, 2, (const uint8_t *)"yo", 2) == 0);
    CHECK(f.nsent == 1 && sent_header(&f, 0).type == SHIM_RELAY_DATA &&
          shim_relay_open(key, 8, f.sent[0], f.sent_len[0]) == SHIM_RELAY_HEADER_SIZE + 8 &&
          f.sent[0][15] == 2 && f.sent[0][16] == 243 && f.sent[0][18] == 'y');

    // Anything not sealed with our key is dropped.
    const uint32_t bad = c.stats.bad_tag;
    from_relay(&c, "other-key", SHIM_RELAY_DATA, 51, 0, in, sizeof(in), 12500);
    CHECK(f.data == 1 && c.stats.bad_tag == bad + 1);

    // RESET: the request in flight fails at once, on_reset runs, and the next poll says HELLO.
    n = shim_relay_eject(p, sizeof(p), 2);
    CHECK(shim_relay_request(&c, SHIM_RELAY_EJECT, p, n, 13000) == 0);
    from_relay(&c, key, SHIM_RELAY_RESET, 0, c.req_seq, NULL, 0, 13010);
    CHECK(f.resets == 1 && c.req_state == SHIM_RELAY_REQ_FAILED && !c.hello_ok);
    f.nsent = 0;
    shim_relay_poll(&c, 13020);
    CHECK(c.req_state == SHIM_RELAY_REQ_PENDING && c.req_type == SHIM_RELAY_HELLO && f.nsent == 1);
}

static void test_sha1(void) {
    uint8_t out[SHIM_SHA1_SIZE];
    shim_sha1("abc", 3, out);
    CHECK(hex_equal(out, 20, "a9993e364706816aba3e25717850c26c9cd0d89d"));
    const char *two = "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"; // 56 bytes: two padding blocks
    shim_sha1(two, strlen(two), out);
    CHECK(hex_equal(out, 20, "84983e441c3bd26ebaae4aa1f95129e5e54670f1"));
    shim_sha1("", 0, out);
    CHECK(hex_equal(out, 20, "da39a3ee5e6b4b0d3255bfef95601890afd80709"));
}

// Finds Nintendo vendor tag `type` in a beacon frame; returns its body from the OUI on.
static const uint8_t *find_tag(const uint8_t *frame, size_t n, uint8_t type, size_t *len) {
    size_t i = 12; // fixed parameters
    while (i + 2 <= n) {
        const uint8_t id = frame[i], l = frame[i + 1];
        if (i + 2 + l > n) {
            break;
        }
        if (id == 0xDD && l >= 4 && frame[i + 2] == 0x00 && frame[i + 3] == 0x1F && frame[i + 4] == 0x32 &&
            frame[i + 5] == type) {
            *len = l;
            return frame + i + 2;
        }
        i += 2 + l;
    }
    return NULL;
}

// A synthesised beacon carries the network struct, app data and a correct SHA-1, and the node
// list it carries comes back out of DecryptBeaconData the way the game would call it.
static void test_beacon(void) {
    uint8_t net[SHIM_UDS_NETWORK_MAX];
    memset(net, 0, sizeof(net));
    uint8_t *ni = net;
    memcpy(ni, "\x02\x11\x22\x33\x44\x01", 6);
    ni[0x06] = 11;
    memcpy(ni + 0x0C, "\x00\x1F\x32\x15", 4);
    memcpy(ni + 0x10, "\x00\x05\x5d\x10", 4);
    ni[0x14] = 1;
    memcpy(ni + 0x18, "\xb4\x13\x55\x38", 4); // network ID
    ni[0x1C] = 2;
    ni[0x1D] = 4;
    ni[0x3F] = 0xB8;
    for (int i = 0; i < 0xB8; i++) {
        ni[0x40 + i] = (uint8_t)(i * 7 + 1);
    }
    net[0x108] = 0x00, net[0x109] = 0x05; // nodes 1 and 3
    uint8_t *n1 = net + 0x10A, *n3 = n1 + 0x28;
    memcpy(n1, "\x01\x02\x03\x04\x05\x06\x07\x08", 8);
    memcpy(n1 + 8, "H\0o\0s\0t\0", 8);
    memcpy(n3, "\x11\x12\x13\x14\x15\x16\x17\x18", 8);
    memcpy(n3 + 8, "J\0o\0i\0n\0e\0r\0", 12);
    shim_uds_network parsed;
    CHECK(shim_uds_parse_network(net, 0x10A + 2 * 0x28, &parsed) == 0);
    CHECK(parsed.bitmask == 5 && parsed.nodes[0] == n1 && parsed.nodes[2] == n3 && !parsed.nodes[1]);
    CHECK(shim_uds_parse_network(net, 0x10A + 0x28 + 5, &parsed) == -1);
    CHECK(shim_uds_parse_network(net, 0x10A + 2 * 0x28, &parsed) == 0);

    uint8_t entry[1024];
    CHECK(shim_uds_beacon_entry(entry, 100, &parsed) == 0); // too small
    const size_t n = shim_uds_beacon_entry(entry, sizeof(entry), &parsed);
    CHECK(n > 0x1C + 12);
    CHECK(entry[0] == (uint8_t)n && entry[5] == 11 && memcmp(entry + 8, ni, 6) == 0 && entry[0x18] == 0x1C);
    const uint8_t *frame = entry + 0x1C;
    const size_t fn = n - 0x1C;

    size_t l;
    const uint8_t *info = find_tag(frame, fn, 0x15, &l);
    CHECK(info != NULL && l == 52 + 0xB8);
    if (!info) {
        return;
    }
    CHECK(memcmp(info, ni + 0x0C, 0x1F) == 0 && info[0x1F + 20] == 0xB8 && memcmp(info + 52, ni + 0x40, 0xB8) == 0);
    uint8_t body[256], hash[20];
    memcpy(body, info, l);
    memset(body + 0x1F, 0, 20);
    shim_sha1(body, l, hash);
    CHECK(memcmp(hash, info + 0x1F, 20) == 0);

    // The game rebuilds the struct from the tag and passes the data tags in 0xFE-byte buffers.
    uint8_t rebuilt[0x108] = {0}, tag0[0xFE] = {0}, tag1[0xFE] = {0};
    memcpy(rebuilt + 0x0C, info, 0x1F);
    size_t l0, l1 = 0;
    const uint8_t *d0 = find_tag(frame, fn, 0x18, &l0);
    const uint8_t *d1 = find_tag(frame, fn, 0x19, &l1);
    CHECK(d0 != NULL && d1 == NULL && l0 == 4 + 0x12 + 4 * 0x1E);
    if (!d0) {
        return;
    }
    memcpy(tag0, d0, l0);
    uint8_t nodes[16 * 0x28];
    shim_uds_decrypt_nodes(rebuilt, tag0, sizeof(tag0), tag1, sizeof(tag1), nodes);
    CHECK(memcmp(nodes, n1, 0x1C) == 0 && nodes[0x20] == 1);
    CHECK(nodes[0x28 + 0x20] == 0 && nodes[0x28] == 0);
    CHECK(memcmp(nodes + 2 * 0x28, n3, 0x1C) == 0 && nodes[2 * 0x28 + 0x20] == 3);

    // Sixteen nodes need the second data tag.
    ni[0x1D] = 16;
    net[0x108] = 0xFF, net[0x109] = 0xFF;
    for (int i = 0; i < 16; i++) {
        uint8_t *node = net + 0x10A + i * 0x28;
        memset(node, 0, 0x28);
        node[0] = (uint8_t)(i + 1);
        node[8] = (uint8_t)('a' + i);
    }
    CHECK(shim_uds_parse_network(net, 0x10A + 16 * 0x28, &parsed) == 0);
    const size_t n16 = shim_uds_beacon_entry(entry, sizeof(entry), &parsed);
    CHECK(n16 > 0);
    d0 = find_tag(entry + 0x1C, n16 - 0x1C, 0x18, &l0);
    d1 = find_tag(entry + 0x1C, n16 - 0x1C, 0x19, &l1);
    CHECK(d0 && d1 && l0 == 4 + 0xFA && l1 == 4 + 0x12 + 16 * 0x1E - 0xFA);
    if (!d0 || !d1) {
        return;
    }
    memset(tag0, 0, sizeof(tag0));
    memset(tag1, 0, sizeof(tag1));
    memcpy(tag0, d0, l0);
    memcpy(tag1, d1, l1);
    memcpy(rebuilt + 0x0C, find_tag(entry + 0x1C, n16 - 0x1C, 0x15, &l), 0x1F);
    shim_uds_decrypt_nodes(rebuilt, tag0, sizeof(tag0), tag1, sizeof(tag1), nodes);
    for (int i = 0; i < 16; i++) {
        CHECK(nodes[i * 0x28] == i + 1 && nodes[i * 0x28 + 8] == 'a' + i && nodes[i * 0x28 + 0x20] == i + 1);
    }
}

int main(int argc, char **argv) {
    const char *fixture = argc > 1 ? argv[1] : NULL;
    test_ipc_parse();
    test_trace_format();
    test_finder();
    test_pick_out_arg();
    test_config();
    test_ring();
    test_session(fixture);
    test_session_error_result();
    test_session_answer();
    test_sha256();
    test_sha1();
    test_beacon();
    test_relay_wire();
    test_relay_client();
    if (failures) {
        fprintf(stderr, "%d check(s) failed\n", failures);
        return 1;
    }
    printf("shim core: all tests passed\n");
    return 0;
}
