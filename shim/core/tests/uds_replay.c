// uds_replay: the shim core's nwm::UDS emulator on a PC, as one console, for trace replay.
// internal/udsfake (ShimConsole) starts one per console and drives it with the requests a game
// would send, against the Go relay over real UDP.
//
//   uds_replay <relay-host> <port> <lobby-key> [label]
//
// A network thread feeds the relay socket to the emulator, as the plugin's relay thread does,
// while the main thread reads one command per line on stdin and answers one line on stdout:
//
//   req <words> <buf>...  An IPC request: words as comma-separated hex, then one hex token per
//                         static buffer descriptor ("-" for none). The harness plays the kernel:
//                         it backs each static or mapped buffer with fake memory, gives copy
//                         handles fake values, and closes them after the reply as the session
//                         loop does. Answers "rsp <words> <buf>...", one hex token per static or
//                         mapped buffer in the reply.
//   peek                  "status <status> <reason> <node> <changed> <total> <max> <bitmask>",
//                         without clearing the changed mask.
//   queued <bind>         "queued <frames>", -1 for no such bind.
//   signals <handle>      "signals <count>" for an event the emulator created.
//   quit                  Exits without leaving any network (like a console losing power). Exit
//                         status 3 if the emulator closed a handle twice or one it did not own.
//
// Emulator log lines go to stderr.

#define _POSIX_C_SOURCE 200809L

#include <arpa/inet.h>
#include <errno.h>
#include <netdb.h>
#include <poll.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

#include "shim/uds.h"

#define MAX_REGIONS 16
#define MAX_HANDLES 4096
#define ADDR_BASE 0x10000000u

typedef struct {
    uint32_t base, size;
    uint8_t *p;
} region;

typedef struct {
    int fd;
    struct sockaddr_storage relay;
    socklen_t relay_len;
    pthread_mutex_t mu;
    pthread_cond_t cv;
    const char *label;
    region regions[MAX_REGIONS];
    int nregions;
    uint32_t next_addr;
    uint32_t next_handle;
    uint8_t handle_open[MAX_HANDLES];
    uint32_t handle_signals[MAX_HANDLES];
    int bad_close;
    pthread_mutex_t handles_mu; // handle table: the network thread signals events too
} harness;

static harness H;
static shim_uds U;
static uint8_t g_out[SHIM_UDS_OUT_SIZE];

static uint64_t now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000 + (uint64_t)ts.tv_nsec / 1000000;
}

static uint64_t o_now(void *ctx) {
    (void)ctx;
    return now_ms();
}
static void o_lock(void *ctx) { pthread_mutex_lock(&((harness *)ctx)->mu); }
static void o_unlock(void *ctx) { pthread_mutex_unlock(&((harness *)ctx)->mu); }
static void o_wait(void *ctx, uint32_t ms) {
    harness *h = ctx;
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    ts.tv_nsec += (long)(ms % 1000) * 1000000;
    ts.tv_sec += ms / 1000 + ts.tv_nsec / 1000000000;
    ts.tv_nsec %= 1000000000;
    pthread_cond_timedwait(&h->cv, &h->mu, &ts);
}
static void o_wake(void *ctx) { pthread_cond_broadcast(&((harness *)ctx)->cv); }
static void o_send(void *ctx, const uint8_t *b, size_t n) {
    harness *h = ctx;
    sendto(h->fd, b, n, 0, (struct sockaddr *)&h->relay, h->relay_len);
}
static shim_handle_t o_create_event(void *ctx) {
    harness *h = ctx;
    pthread_mutex_lock(&h->handles_mu);
    const uint32_t v = h->next_handle < MAX_HANDLES ? h->next_handle++ : 0;
    if (v) {
        h->handle_open[v] = 1;
        h->handle_signals[v] = 0;
    }
    pthread_mutex_unlock(&h->handles_mu);
    return v;
}
static void o_signal(void *ctx, shim_handle_t v) {
    harness *h = ctx;
    pthread_mutex_lock(&h->handles_mu);
    if (v < MAX_HANDLES && h->handle_open[v]) {
        h->handle_signals[v]++;
    } else {
        fprintf(stderr, "[%s] harness: signal of handle 0x%x that is not open\n", h->label, (unsigned)v);
        h->bad_close++;
    }
    pthread_mutex_unlock(&h->handles_mu);
}
static void o_close(void *ctx, shim_handle_t v) {
    harness *h = ctx;
    pthread_mutex_lock(&h->handles_mu);
    if (v < MAX_HANDLES && h->handle_open[v]) {
        h->handle_open[v] = 0;
    } else {
        fprintf(stderr, "[%s] harness: close of handle 0x%x that is not open\n", h->label, (unsigned)v);
        h->bad_close++;
    }
    pthread_mutex_unlock(&h->handles_mu);
}

static region *find_region_ptr(harness *h, const void *p) {
    const uint8_t *b = p;
    for (int i = 0; i < h->nregions; i++) {
        region *r = &h->regions[i];
        if (b >= r->p && b < r->p + r->size) {
            return r;
        }
    }
    return NULL;
}

static uint32_t o_addr_of(void *ctx, const void *p) {
    harness *h = ctx;
    region *r = find_region_ptr(h, p);
    if (!r) {
        fprintf(stderr, "[%s] harness: addr_of a pointer outside every region\n", h->label);
        abort();
    }
    return r->base + (uint32_t)((const uint8_t *)p - r->p);
}

static uint8_t *o_mem(void *ctx, uint32_t addr, uint32_t size) {
    harness *h = ctx;
    for (int i = 0; i < h->nregions; i++) {
        region *r = &h->regions[i];
        if (addr >= r->base && addr - r->base + (uint64_t)size <= r->size) {
            return r->p + (addr - r->base);
        }
    }
    return NULL;
}

static void o_log(void *ctx, const char *level, const char *msg) {
    harness *h = ctx;
    fprintf(stderr, "[%s] %s: %s\n", h->label, level, msg);
}

static const shim_uds_ops kOps = {&H,     o_now,      o_lock,     o_unlock,     o_wait,   o_wake,
                                  o_send, o_create_event, o_signal, o_close, o_addr_of, o_mem, o_log};

// Fake memory for one request's buffers. Region 0 is g_out and stays.
static region *add_region(harness *h, uint8_t *p, uint32_t size) {
    if (h->nregions == MAX_REGIONS) {
        return NULL;
    }
    region *r = &h->regions[h->nregions++];
    r->p = p;
    r->size = size ? size : 1;
    r->base = h->next_addr;
    h->next_addr += (r->size + 0xFFF) & ~0xFFFu;
    return r;
}

static void drop_request_regions(harness *h) {
    for (int i = 1; i < h->nregions; i++) {
        free(h->regions[i].p);
    }
    h->nregions = 1;
}

static int unhex(const char *s, uint8_t *out, size_t cap) {
    const size_t n = strlen(s);
    if (n % 2 || n / 2 > cap) {
        return -1;
    }
    for (size_t i = 0; i < n / 2; i++) {
        unsigned v;
        if (sscanf(s + 2 * i, "%2x", &v) != 1) {
            return -1;
        }
        out[i] = (uint8_t)v;
    }
    return (int)(n / 2);
}

static void put_hex(const uint8_t *b, size_t n) {
    static const char digits[] = "0123456789abcdef";
    if (!n) {
        fputs(" -", stdout);
        return;
    }
    putchar(' ');
    for (size_t i = 0; i < n; i++) {
        putchar(digits[b[i] >> 4]);
        putchar(digits[b[i] & 0xF]);
    }
}

static void request(harness *h, char *args) {
    uint32_t cmd[SHIM_IPC_MAX_WORDS] = {0};
    char *save = NULL;
    char *words = strtok_r(args, " ", &save);
    int nw = 0;
    for (char *w = words ? strtok(words, ",") : NULL; w && nw < SHIM_IPC_MAX_WORDS; w = strtok(NULL, ",")) {
        cmd[nw++] = (uint32_t)strtoul(w, NULL, 16);
    }
    shim_ipc_msg m;
    if (nw == 0 || (unsigned)nw < shim_ipc_words(cmd[0]) || shim_ipc_parse(cmd, &m) != 0) {
        printf("err bad request\n");
        return;
    }
    shim_handle_t copies[16];
    int ncopies = 0;
    for (unsigned i = 0; i < m.ndesc; i++) {
        const shim_desc *d = &m.desc[i];
        if (d->kind == SHIM_DESC_STATIC_BUFFER || d->kind == SHIM_DESC_MAPPED_BUFFER) {
            uint8_t *p = calloc(1, d->size ? d->size : 1);
            if (d->kind == SHIM_DESC_STATIC_BUFFER) {
                const char *tok = strtok_r(NULL, " ", &save);
                if (tok && strcmp(tok, "-") != 0 && unhex(tok, p, d->size) < 0) {
                    printf("err static buffer %u does not fit its descriptor\n", i);
                    free(p);
                    drop_request_regions(h);
                    return;
                }
            }
            region *r = add_region(h, p, d->size);
            cmd[d->word + 1] = r->base;
        } else if (d->kind == SHIM_DESC_COPY_HANDLES) {
            for (unsigned k = 0; k < d->count; k++) {
                const shim_handle_t v = o_create_event(h); // stands in for the game's handle
                cmd[d->word + 1 + k] = v;
                copies[ncopies++] = v;
            }
        }
    }

    shim_uds_handle(&U, cmd, g_out);

    for (int i = 0; i < ncopies; i++) {
        o_close(h, copies[i]); // the session loop closes our duplicates of request handles
    }
    shim_ipc_msg r;
    if (shim_ipc_parse(cmd, &r) != 0) {
        printf("err malformed reply 0x%08x\n", (unsigned)cmd[0]);
        drop_request_regions(h);
        return;
    }
    printf("rsp ");
    for (unsigned i = 0; i < shim_ipc_words(cmd[0]); i++) {
        printf(i ? ",%08x" : "%08x", (unsigned)cmd[i]);
    }
    for (unsigned i = 0; i < r.ndesc; i++) {
        const shim_desc *d = &r.desc[i];
        if (d->kind != SHIM_DESC_STATIC_BUFFER && d->kind != SHIM_DESC_MAPPED_BUFFER) {
            continue;
        }
        const uint8_t *p = o_mem(h, d->ptr, d->size);
        if (!p && d->size) {
            fprintf(stderr, "[%s] harness: reply buffer 0x%08x+0x%x is not mapped\n", h->label, (unsigned)d->ptr,
                    (unsigned)d->size);
            h->bad_close++;
        }
        put_hex(p, p ? d->size : 0);
    }
    putchar('\n');
    drop_request_regions(h);
}

static void *net_thread(void *arg) {
    (void)arg;
    uint64_t last_poll = 0;
    for (;;) {
        struct pollfd pfd = {H.fd, POLLIN, 0};
        if (poll(&pfd, 1, 20) > 0 && (pfd.revents & POLLIN)) {
            uint8_t buf[SHIM_RELAY_MAX_DATAGRAM];
            const ssize_t n = recv(H.fd, buf, sizeof(buf), 0);
            if (n > 0) {
                shim_uds_net_input(&U, buf, (size_t)n);
            }
        }
        if (now_ms() - last_poll >= 20) {
            shim_uds_net_poll(&U);
            last_poll = now_ms();
        }
    }
    return NULL;
}

int main(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "usage: uds_replay <relay-host> <port> <lobby-key> [label]\n");
        return 2;
    }
    H.label = argc > 4 ? argv[4] : "console";
    struct addrinfo hints = {0}, *ai;
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_DGRAM;
    if (getaddrinfo(argv[1], argv[2], &hints, &ai) != 0) {
        fprintf(stderr, "cannot resolve %s\n", argv[1]);
        return 2;
    }
    H.fd = socket(ai->ai_family, SOCK_DGRAM, 0);
    memcpy(&H.relay, ai->ai_addr, ai->ai_addrlen);
    H.relay_len = ai->ai_addrlen;
    freeaddrinfo(ai);
    pthread_mutex_init(&H.mu, NULL);
    pthread_cond_init(&H.cv, NULL);
    pthread_mutex_init(&H.handles_mu, NULL);
    H.next_addr = ADDR_BASE;
    H.next_handle = 0x100;
    add_region(&H, g_out, sizeof(g_out));

    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    const uint32_t session = ((uint32_t)ts.tv_nsec ^ (uint32_t)getpid() << 16 ^ (uint32_t)ts.tv_sec) | 1;
    if (shim_uds_init(&U, &kOps, argv[3], strlen(argv[3]), session) != 0) {
        fprintf(stderr, "bad lobby key\n");
        return 2;
    }
    pthread_t t;
    pthread_create(&t, NULL, net_thread, NULL);

    char *line = NULL;
    size_t cap = 0;
    ssize_t n;
    setvbuf(stdout, NULL, _IOFBF, 1 << 16);
    while ((n = getline(&line, &cap, stdin)) > 0) {
        while (n > 0 && (line[n - 1] == '\n' || line[n - 1] == '\r')) {
            line[--n] = '\0';
        }
        if (strncmp(line, "req ", 4) == 0) {
            request(&H, line + 4);
        } else if (strcmp(line, "peek") == 0) {
            shim_uds_status st;
            shim_uds_peek(&U, &st);
            printf("status %u %u %u %u %u %u %u\n", (unsigned)st.status, (unsigned)st.reason, (unsigned)st.node_id,
                   (unsigned)st.changed, (unsigned)st.total, (unsigned)st.max, (unsigned)st.bitmask);
        } else if (strncmp(line, "queued ", 7) == 0) {
            printf("queued %d\n", shim_uds_queued(&U, (uint32_t)strtoul(line + 7, NULL, 0)));
        } else if (strncmp(line, "signals ", 8) == 0) {
            const unsigned long v = strtoul(line + 8, NULL, 0);
            pthread_mutex_lock(&H.handles_mu);
            printf("signals %u\n", v < MAX_HANDLES ? (unsigned)H.handle_signals[v] : 0u);
            pthread_mutex_unlock(&H.handles_mu);
        } else if (strcmp(line, "quit") == 0) {
            break;
        } else {
            printf("err unknown command\n");
        }
        fflush(stdout);
    }
    free(line);
    pthread_mutex_lock(&H.handles_mu);
    const int bad = H.bad_close;
    pthread_mutex_unlock(&H.handles_mu);
    if (bad) {
        fprintf(stderr, "[%s] harness: %d bad handle operations\n", H.label, bad);
    }
    fflush(stdout);
    _exit(bad ? 3 : 0); // the network thread never returns
}
