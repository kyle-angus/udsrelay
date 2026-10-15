// relay_probe: the shim core's relay client on a PC, over a real UDP socket, playing a hosting
// console. internal/relay/probe_test.go builds it, runs it against an in-process Go relay and
// joins from the Go client, so the C and Go sides are tested against each other end to end.
//
//   relay_probe <relay-host> <port> <lobby-key> <frames>
//
// It says HELLO, hosts a Pokémon X/Y-shaped trade network (comm ID 0x00055d10, ID8 1, 4 nodes),
// waits for node 2 to join, sends <frames> frames "c2g-<i>" to it, expects <frames> frames
// "g2c-<i>" back, and waits for node 2 to leave. It prints one line per step and exits 0 on
// success, 1 on any failure or after 10 s.

#define _POSIX_C_SOURCE 200809L

#include <arpa/inet.h>
#include <netdb.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

#include "shim/relay_client.h"
#include "shim/sha256.h"

typedef struct {
    int fd;
    struct sockaddr_storage relay;
    socklen_t relay_len;
    int joined, left, received;
    char expect[32];
    int frames;
} probe;

static uint64_t now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000 + (uint64_t)ts.tv_nsec / 1000000;
}

static void p_send(void *ctx, const uint8_t *b, size_t n) {
    probe *p = (probe *)ctx;
    sendto(p->fd, b, n, 0, (struct sockaddr *)&p->relay, p->relay_len);
}

static void p_data(void *ctx, uint16_t src, uint16_t dst, uint8_t ch, uint8_t fl, const uint8_t *b, size_t n) {
    probe *p = (probe *)ctx;
    char want[32];
    snprintf(want, sizeof(want), "g2c-%d", p->received);
    if (src == 2 && dst == 1 && ch == 243 && fl == 2 && n == strlen(want) && memcmp(b, want, n) == 0) {
        p->received++;
    } else {
        printf("unexpected frame from node %u: %.*s\n", (unsigned)src, (int)n, (const char *)b);
    }
}

static void p_event(void *ctx, const shim_relay_event *e) {
    probe *p = (probe *)ctx;
    printf("event kind %u node %u bitmask 0x%04x total %u\n", e->kind, e->node, e->bitmask, e->total);
    if (e->kind == SHIM_RELAY_EV_JOINED && e->node == 2) {
        p->joined = 1;
    }
    if (e->kind == SHIM_RELAY_EV_LEFT && e->node == 2) {
        p->left = 1;
    }
}

static void p_reset(void *ctx) {
    (void)ctx;
    printf("relay reset our session\n");
}

// Runs the client until the request in flight finishes. Returns 0 if it got status OK.
static int wait_request(shim_relay_client *c, probe *p, uint64_t deadline) {
    while (c->req_state == SHIM_RELAY_REQ_PENDING && now_ms() < deadline) {
        fd_set fds;
        FD_ZERO(&fds);
        FD_SET(p->fd, &fds);
        struct timeval tv = {0, 20000};
        if (select(p->fd + 1, &fds, NULL, NULL, &tv) > 0) {
            uint8_t buf[SHIM_RELAY_MAX_DATAGRAM];
            const ssize_t n = recv(p->fd, buf, sizeof(buf), 0);
            if (n > 0) {
                shim_relay_input(c, buf, (size_t)n, now_ms());
            }
        }
        shim_relay_poll(c, now_ms());
    }
    const int ok = c->req_state == SHIM_RELAY_REQ_DONE && c->status == SHIM_RELAY_STATUS_OK;
    shim_relay_request_clear(c);
    return ok ? 0 : -1;
}

// Runs the client until cond() holds or the deadline passes.
static int run_until(shim_relay_client *c, probe *p, uint64_t deadline, int (*cond)(probe *)) {
    while (!cond(p) && now_ms() < deadline) {
        fd_set fds;
        FD_ZERO(&fds);
        FD_SET(p->fd, &fds);
        struct timeval tv = {0, 20000};
        if (select(p->fd + 1, &fds, NULL, NULL, &tv) > 0) {
            uint8_t buf[SHIM_RELAY_MAX_DATAGRAM];
            const ssize_t n = recv(p->fd, buf, sizeof(buf), 0);
            if (n > 0) {
                shim_relay_input(c, buf, (size_t)n, now_ms());
            }
        }
        shim_relay_poll(c, now_ms());
    }
    return cond(p) ? 0 : -1;
}

static int is_joined(probe *p) { return p->joined; }
static int got_all(probe *p) { return p->received >= p->frames; }
static int has_left(probe *p) { return p->left; }

int main(int argc, char **argv) {
    if (argc != 5) {
        fprintf(stderr, "usage: relay_probe <relay-host> <port> <lobby-key> <frames>\n");
        return 2;
    }
    probe p;
    memset(&p, 0, sizeof(p));
    p.frames = atoi(argv[4]);
    struct addrinfo hints = {0}, *ai;
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_DGRAM;
    if (getaddrinfo(argv[1], argv[2], &hints, &ai) != 0) {
        fprintf(stderr, "cannot resolve %s\n", argv[1]);
        return 2;
    }
    p.fd = socket(ai->ai_family, SOCK_DGRAM, 0);
    memcpy(&p.relay, ai->ai_addr, ai->ai_addrlen);
    p.relay_len = ai->ai_addrlen;
    freeaddrinfo(ai);

    const shim_relay_ops ops = {&p, p_send, p_data, p_event, p_reset, NULL};
    shim_relay_client c;
    srand((unsigned)now_ms());
    const uint32_t session = ((uint32_t)rand() << 16 ^ (uint32_t)rand()) | 1;
    if (shim_relay_client_init(&c, &ops, argv[3], strlen(argv[3]), session) != 0) {
        fprintf(stderr, "bad lobby key\n");
        return 2;
    }
    const uint64_t deadline = now_ms() + 10000;

    shim_relay_hello_request(&c, now_ms());
    if (wait_request(&c, &p, deadline) != 0) {
        printf("FAIL: HELLO not answered\n");
        return 1;
    }
    printf("hello ok, keepalive %u ms\n", (unsigned)c.keepalive_ms);

    uint8_t netinfo[SHIM_RELAY_NETINFO_SIZE] = {0}, self[SHIM_RELAY_NODEINFO_SIZE] = {0}, hash[32];
    netinfo[0x10] = 0x00, netinfo[0x11] = 0x05, netinfo[0x12] = 0x5d, netinfo[0x13] = 0x10; // comm ID
    netinfo[0x14] = 0x01;                                                                    // ID8
    netinfo[0x1D] = 4;                                                                       // max nodes
    memcpy(self + 8, "p\0r\0o\0b\0e\0", 10);
    shim_sha256("GNgBwMcVotYs", 13, hash); // with its NUL, as X/Y passes it
    uint8_t req[512];
    size_t n = shim_relay_host_create(req, sizeof(req), hash, self, netinfo);
    shim_relay_request(&c, SHIM_RELAY_HOST_CREATE, req, n, now_ms());
    if (wait_request(&c, &p, deadline) != 0 || c.resp_len < 4) {
        printf("FAIL: HOST_CREATE\n");
        return 1;
    }
    printf("hosting network 0x%08x\n",
           (unsigned)((uint32_t)c.resp[0] << 24 | (uint32_t)c.resp[1] << 16 | (uint32_t)c.resp[2] << 8 | c.resp[3]));

    if (run_until(&c, &p, deadline, is_joined) != 0) {
        printf("FAIL: nobody joined\n");
        return 1;
    }
    for (int i = 0; i < p.frames; i++) {
        char f[32];
        snprintf(f, sizeof(f), "c2g-%d", i);
        shim_relay_send_data(&c, 2, 243, 2, (const uint8_t *)f, strlen(f));
    }
    if (run_until(&c, &p, deadline, got_all) != 0) {
        printf("FAIL: received %d of %d frames\n", p.received, p.frames);
        return 1;
    }
    printf("received %d frames\n", p.received);
    if (run_until(&c, &p, deadline, has_left) != 0) {
        printf("FAIL: node 2 never left\n");
        return 1;
    }
    printf("PASS rtt %u ms, retries %u, bad tags %u\n", (unsigned)c.rtt_ms, (unsigned)c.stats.retries,
           (unsigned)c.stats.bad_tag);
    close(p.fd);
    return 0;
}
