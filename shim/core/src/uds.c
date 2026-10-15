#include "shim/uds.h"

#include <stdarg.h>
#include <stdio.h>
#include <string.h>

#include "shim/sha1.h"
#include "shim/sha256.h"

#define NONE 0xFFFF
#define HELLO_WAIT_MS 3000  // InitializeWithVersion waits this long for the relay
#define HELLO_RETRY_MS 2000 // between HELLOs while the relay does not answer
#define SLOT_WAIT_MS 5000   // for another session's relay request to finish
#define DEFAULT_BIND_BUF 0x2e30

// Network struct offsets (3dbrew NWM_UDS; multi-byte fields big-endian).
#define NI_CHANNEL 0x06
#define NI_OUI 0x0C
#define NI_OUI_TYPE 0x0F
#define NI_NETWORK_ID 0x18
#define NI_TOTAL 0x1C
#define NI_MAX 0x1D
#define NI_APPDATA_SIZE 0x3F
#define NI_APPDATA 0x40
#define MAX_APPDATA 0xC8
#define NODE_ID_OFFSET 0x20 // NodeInfo.NetworkNodeID

static const uint8_t kOUI[3] = {0x00, 0x1F, 0x32};
enum { TAG_DUMMY = 0x14, TAG_NETINFO = 0x15, TAG_DATA0 = 0x18, TAG_DATA1 = 0x19, TAG_VENDOR = 0xDD };

static void put16be(uint8_t *p, uint16_t v) { p[0] = (uint8_t)(v >> 8), p[1] = (uint8_t)v; }
static void put32be(uint8_t *p, uint32_t v) { put16be(p, (uint16_t)(v >> 16)), put16be(p + 2, (uint16_t)v); }
static uint16_t get16be(const uint8_t *p) { return (uint16_t)(p[0] << 8 | p[1]); }
static uint32_t get32be(const uint8_t *p) { return (uint32_t)get16be(p) << 16 | get16be(p + 2); }
static void put16le(uint8_t *p, uint16_t v) { p[0] = (uint8_t)v, p[1] = (uint8_t)(v >> 8); }
static void put32le(uint8_t *p, uint32_t v) { put16le(p, (uint16_t)v), put16le(p + 2, (uint16_t)(v >> 16)); }
static uint16_t get16le(const uint8_t *p) { return (uint16_t)(p[0] | p[1] << 8); }

static uint64_t now(shim_uds *u) { return u->ops->now_ms(u->ops->ctx); }

static void note(shim_uds *u, const char *level, const char *fmt, ...) __attribute__((format(printf, 3, 4)));
static void note(shim_uds *u, const char *level, const char *fmt, ...) {
    if (!u->ops->log) {
        return;
    }
    char msg[200];
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(msg, sizeof(msg), fmt, ap);
    va_end(ap);
    u->ops->log(u->ops->ctx, level, msg);
}

// ---- beacons --------------------------------------------------------------------------------

int shim_uds_parse_network(const uint8_t *b, size_t n, shim_uds_network *out) {
    memset(out, 0, sizeof(*out));
    if (n < SHIM_RELAY_NETINFO_SIZE + 2) {
        return -1;
    }
    out->netinfo = b;
    out->bitmask = get16be(b + SHIM_RELAY_NETINFO_SIZE);
    size_t off = SHIM_RELAY_NETINFO_SIZE + 2;
    for (int i = 0; i < SHIM_RELAY_MAX_NODES; i++) {
        if (!(out->bitmask & (1u << i))) {
            continue;
        }
        if (off + SHIM_RELAY_NODEINFO_SIZE > n) {
            return -1;
        }
        out->nodes[i] = b + off;
        off += SHIM_RELAY_NODEINFO_SIZE;
    }
    return 0;
}

static unsigned max_nodes(const uint8_t *netinfo) {
    const unsigned m = netinfo[NI_MAX];
    return m == 0 || m > SHIM_RELAY_MAX_NODES ? SHIM_RELAY_MAX_NODES : m;
}

// Azahar's BeaconData (MD5, u16 bitmask) and one BeaconNodeInfo per node slot (u64 seed,
// UTF-16 username, u16 node ID; all big-endian). Left unencrypted: only our DecryptBeaconData
// reads it. Returns the length.
static size_t beacon_node_data(uint8_t *out, const shim_uds_network *net) {
    const unsigned slots = max_nodes(net->netinfo);
    memset(out, 0, 0x12 + slots * 0x1E);
    put16be(out + 0x10, net->bitmask);
    for (unsigned i = 0; i < slots; i++) {
        uint8_t *e = out + 0x12 + i * 0x1E;
        const uint8_t *node = net->nodes[i];
        if (!node) {
            continue;
        }
        for (int k = 0; k < 8; k++) {
            e[k] = node[7 - k]; // little-endian seed to big-endian
        }
        for (int k = 0; k < 10; k++) {
            put16be(e + 8 + 2 * k, get16le(node + 8 + 2 * k));
        }
        put16be(e + 0x1C, (uint16_t)(i + 1));
    }
    return 0x12 + slots * 0x1E;
}

size_t shim_uds_beacon_entry(uint8_t *out, size_t cap, const shim_uds_network *net) {
    uint8_t frame[1024];
    size_t f = 0;
    const uint8_t *ni = net->netinfo;

    // Fixed parameters: timestamp (15 minutes up), beacon interval 100 TU, capabilities.
    memset(frame, 0, 12);
    const uint64_t uptime = 900000000;
    for (int i = 0; i < 8; i++) {
        frame[i] = (uint8_t)(uptime >> (8 * i));
    }
    put16le(frame + 8, 100);
    put16le(frame + 10, 0x0431);
    f = 12;

    // SSID: 8 zero bytes.
    frame[f++] = 0x00;
    frame[f++] = 8;
    memset(frame + f, 0, 8);
    f += 8;

    // Nintendo dummy tag, as captured from an o3DS.
    const uint8_t dummy[] = {TAG_VENDOR, 7, 0x00, 0x1F, 0x32, TAG_DUMMY, 0x0A, 0x00, 0x00};
    memcpy(frame + f, dummy, sizeof(dummy));
    f += sizeof(dummy);

    // Network info: the struct from the OUI on (0x1F bytes), SHA-1, app data size, app data.
    unsigned appsize = ni[NI_APPDATA_SIZE];
    if (appsize > MAX_APPDATA) {
        appsize = MAX_APPDATA;
    }
    uint8_t *tag = frame + f;
    tag[0] = TAG_VENDOR;
    tag[1] = (uint8_t)(52 + appsize);
    memcpy(tag + 2, ni + NI_OUI, 0x1F);
    memcpy(tag + 2, kOUI, 3);
    tag[2 + 3] = TAG_NETINFO;
    memset(tag + 2 + 0x1F, 0, SHIM_SHA1_SIZE);
    tag[2 + 0x1F + SHIM_SHA1_SIZE] = (uint8_t)appsize;
    memcpy(tag + 54, ni + NI_APPDATA, appsize);
    uint8_t hash[SHIM_SHA1_SIZE];
    shim_sha1(tag + 2, 52 + appsize, hash);
    memcpy(tag + 2 + 0x1F, hash, SHIM_SHA1_SIZE);
    f += 54 + appsize;

    // Node list, split over two data tags at 0xFA bytes.
    uint8_t data[0x12 + SHIM_RELAY_MAX_NODES * 0x1E];
    const size_t dn = beacon_node_data(data, net);
    const size_t first = dn < 0xFA ? dn : 0xFA;
    const uint8_t t0[] = {TAG_VENDOR, (uint8_t)(4 + first), 0x00, 0x1F, 0x32, TAG_DATA0};
    memcpy(frame + f, t0, sizeof(t0));
    memcpy(frame + f + sizeof(t0), data, first);
    f += sizeof(t0) + first;
    if (dn > first) {
        const uint8_t t1[] = {TAG_VENDOR, (uint8_t)(4 + dn - first), 0x00, 0x1F, 0x32, TAG_DATA1};
        memcpy(frame + f, t1, sizeof(t1));
        memcpy(frame + f + sizeof(t1), data + first, dn - first);
        f += sizeof(t1) + dn - first;
    }

    // BeaconEntryHeader.
    const size_t total = 0x1C + f;
    if (total > cap) {
        return 0;
    }
    memset(out, 0, 0x1C);
    put32le(out, (uint32_t)total);
    out[5] = ni[NI_CHANNEL];
    memcpy(out + 8, ni, 6); // host MAC
    put32le(out + 0x14, (uint32_t)total);
    put32le(out + 0x18, 0x1C);
    memcpy(out + 0x1C, frame, f);
    return total;
}

void shim_uds_decrypt_nodes(const uint8_t netinfo[SHIM_RELAY_NETINFO_SIZE], const uint8_t *tag0, size_t n0,
                            const uint8_t *tag1, size_t n1, uint8_t out[SHIM_RELAY_MAX_NODES * SHIM_RELAY_NODEINFO_SIZE]) {
    memset(out, 0, SHIM_RELAY_MAX_NODES * SHIM_RELAY_NODEINFO_SIZE);
    if (!tag0 || n0 < 4 || memcmp(tag0, kOUI, 3) != 0 || tag0[3] != TAG_DATA0) {
        return;
    }
    uint8_t data[2 * 0xFF];
    size_t dn = n0 - 4 < 0xFF ? n0 - 4 : 0xFF;
    memcpy(data, tag0 + 4, dn);
    if (tag1 && n1 >= 4 && memcmp(tag1, kOUI, 3) == 0 && tag1[3] == TAG_DATA1) {
        const size_t m = n1 - 4 < 0xFF ? n1 - 4 : 0xFF;
        memcpy(data + dn, tag1 + 4, m);
        dn += m;
    }
    const unsigned slots = max_nodes(netinfo);
    for (unsigned i = 0; i < slots && 0x12 + (i + 1) * 0x1E <= dn; i++) {
        const uint8_t *e = data + 0x12 + i * 0x1E;
        uint8_t *node = out + i * SHIM_RELAY_NODEINFO_SIZE;
        for (int k = 0; k < 8; k++) {
            node[k] = e[7 - k];
        }
        for (int k = 0; k < 10; k++) {
            put16le(node + 8 + 2 * k, get16be(e + 8 + 2 * k));
        }
        put16le(node + NODE_ID_OFFSET, get16be(e + 0x1C));
    }
}

// ---- state ----------------------------------------------------------------------------------

static int in_network(const shim_uds *u) {
    return u->st.status == SHIM_UDS_HOST || u->st.status == SHIM_UDS_CLIENT || u->st.status == SHIM_UDS_SPECTATOR;
}

static void signal_status(shim_uds *u) {
    u->stats.status_signals++;
    if (u->status_event) {
        u->ops->signal_event(u->ops->ctx, u->status_event);
    }
}

static void not_connected(shim_uds *u, uint32_t reason) {
    const uint16_t node = u->st.node_id;
    memset(&u->st, 0, sizeof(u->st));
    u->st.status = SHIM_UDS_NOT_CONNECTED;
    u->st.reason = reason;
    u->st.node_id = node;
    signal_status(u);
}

static void free_queue(shim_uds *u, shim_uds_bind *b) {
    while (b->head != NONE) {
        const uint16_t i = b->head;
        b->head = u->packets[i].next;
        u->packets[i].next = u->free_head;
        u->free_head = i;
    }
    b->tail = NONE;
    b->count = b->queued_bytes = 0;
}

static void drop_bind(shim_uds *u, shim_uds_bind *b) {
    free_queue(u, b);
    if (b->event) {
        u->ops->close_handle(u->ops->ctx, b->event);
    }
    memset(b, 0, sizeof(*b));
    b->head = b->tail = NONE;
}

static void drop_binds(shim_uds *u) {
    for (int i = 0; i < SHIM_UDS_MAX_BINDS; i++) {
        if (u->binds[i].used) {
            drop_bind(u, &u->binds[i]);
        }
    }
}

static shim_uds_bind *find_bind(shim_uds *u, uint32_t id) {
    for (int i = 0; i < SHIM_UDS_MAX_BINDS; i++) {
        if (u->binds[i].used && u->binds[i].id == id) {
            return &u->binds[i];
        }
    }
    return NULL;
}

// ---- relay callbacks (network thread, lock held) --------------------------------------------

static void r_send(void *ctx, const uint8_t *b, size_t n) {
    shim_uds *u = (shim_uds *)ctx;
    u->ops->send(u->ops->ctx, b, n);
}

static void r_data(void *ctx, uint16_t src, uint16_t dst, uint8_t channel, uint8_t flags, const uint8_t *p,
                   size_t n) {
    shim_uds *u = (shim_uds *)ctx;
    (void)dst, (void)flags;
    if (!in_network(u) || n > SHIM_UDS_MAX_FRAME) {
        u->stats.dropped++;
        return;
    }
    int delivered = 0;
    for (int i = 0; i < SHIM_UDS_MAX_BINDS; i++) {
        shim_uds_bind *b = &u->binds[i];
        if (!b->used || b->channel != channel || (b->node != SHIM_RELAY_BROADCAST && b->node != src)) {
            continue;
        }
        if (b->queued_bytes + n > b->buf_size || u->free_head == NONE) {
            continue;
        }
        const uint16_t s = u->free_head;
        shim_uds_packet *pk = &u->packets[s];
        u->free_head = pk->next;
        pk->next = NONE;
        pk->src = src;
        pk->len = (uint16_t)n;
        memcpy(pk->data, p, n);
        if (b->tail == NONE) {
            b->head = s;
        } else {
            u->packets[b->tail].next = s;
        }
        b->tail = s;
        b->count++;
        b->queued_bytes += (uint32_t)n;
        b->received++;
        u->stats.bind_signals++;
        if (b->event) {
            u->ops->signal_event(u->ops->ctx, b->event);
        }
        delivered = 1;
    }
    if (!delivered) {
        u->stats.dropped++;
    }
}

static void r_event(void *ctx, const shim_relay_event *e) {
    shim_uds *u = (shim_uds *)ctx;
    if (!in_network(u) || e->network_id != get32be(u->netinfo + NI_NETWORK_ID)) {
        return; // a network we already left
    }
    switch (e->kind) {
    case SHIM_RELAY_EV_JOINED:
    case SHIM_RELAY_EV_LEFT: {
        uint16_t bit = 0;
        if (e->node >= 1 && e->node <= SHIM_RELAY_MAX_NODES) {
            bit = (uint16_t)(1u << (e->node - 1));
            if (e->kind == SHIM_RELAY_EV_JOINED) {
                memcpy(u->nodes[e->node - 1], e->info, SHIM_RELAY_NODEINFO_SIZE);
            } else {
                memset(u->nodes[e->node - 1], 0, SHIM_RELAY_NODEINFO_SIZE);
            }
        }
        u->st.bitmask = e->bitmask;
        u->st.total = e->total;
        u->st.changed |= bit;
        u->netinfo[NI_TOTAL] = e->total;
        note(u, "Info", "node %u %s; bitmask 0x%04x", (unsigned)e->node,
             e->kind == SHIM_RELAY_EV_JOINED ? "joined" : "left", (unsigned)e->bitmask);
        signal_status(u);
        break;
    }
    case SHIM_RELAY_EV_EJECTED:
    case SHIM_RELAY_EV_CLOSED:
        note(u, "Info", "network 0x%08lx %s", (unsigned long)e->network_id,
             e->kind == SHIM_RELAY_EV_EJECTED ? "ejected us" : "closed");
        not_connected(u, SHIM_UDS_REASON_LOST);
        break;
    }
}

static void r_reset(void *ctx) {
    shim_uds *u = (shim_uds *)ctx;
    note(u, "Warning", "relay reset our session");
    if (in_network(u)) {
        not_connected(u, SHIM_UDS_REASON_LOST);
    }
}

static void r_scan(void *ctx, uint8_t index, uint8_t total, const uint8_t *network, size_t n) {
    shim_uds *u = (shim_uds *)ctx;
    (void)index, (void)total;
    if (u->nscan < SHIM_UDS_MAX_SCAN && n <= SHIM_UDS_NETWORK_MAX) {
        memcpy(u->scan[u->nscan], network, n);
        u->scan_len[u->nscan] = (uint16_t)n;
        u->nscan++;
    }
}

// ---- public entry points --------------------------------------------------------------------

int shim_uds_init(shim_uds *u, const shim_uds_ops *ops, const char *key, size_t key_len, uint32_t session) {
    memset(u, 0, sizeof(*u));
    u->ops = ops;
    u->relay_ops.ctx = u;
    u->relay_ops.send = r_send;
    u->relay_ops.on_data = r_data;
    u->relay_ops.on_event = r_event;
    u->relay_ops.on_reset = r_reset;
    u->relay_ops.on_scan = r_scan;
    if (shim_relay_client_init(&u->relay, &u->relay_ops, key, key_len, session) != 0) {
        return -1;
    }
    u->st.status = SHIM_UDS_NOT_CONNECTED;
    for (int i = 0; i < SHIM_UDS_MAX_BINDS; i++) {
        u->binds[i].head = u->binds[i].tail = NONE;
    }
    for (int i = 0; i < SHIM_UDS_PACKET_SLOTS; i++) {
        u->packets[i].next = (uint16_t)(i + 1 < SHIM_UDS_PACKET_SLOTS ? i + 1 : NONE);
    }
    u->free_head = 0;
    // A locally administered MAC for the beacons, from the session ID.
    const uint8_t mac[6] = {0x02, (uint8_t)(session >> 24), (uint8_t)(session >> 16), (uint8_t)(session >> 8),
                            (uint8_t)session, 0x01};
    memcpy(u->mac, mac, sizeof(mac));
    return 0;
}

void shim_uds_net_input(shim_uds *u, const uint8_t *b, size_t n) {
    u->ops->lock(u->ops->ctx);
    shim_relay_input(&u->relay, b, n, now(u));
    u->ops->wake(u->ops->ctx);
    u->ops->unlock(u->ops->ctx);
}

void shim_uds_net_poll(shim_uds *u) {
    u->ops->lock(u->ops->ctx);
    const uint64_t t = now(u);
    shim_relay_poll(&u->relay, t);
    if (!u->busy && u->relay.req_state != SHIM_RELAY_REQ_PENDING) {
        if (u->relay.req_type == SHIM_RELAY_HELLO && u->relay.req_state != SHIM_RELAY_REQ_IDLE) {
            if (u->relay.req_state == SHIM_RELAY_REQ_FAILED || u->relay.status != SHIM_RELAY_STATUS_OK) {
                note(u, "Warning", "relay did not answer HELLO%s",
                     u->relay.stats.bad_tag ? " (answers did not verify: wrong lobby key?)" : "");
            } else {
                note(u, "Info", "relay answered HELLO; keepalive %lu ms", (unsigned long)u->relay.keepalive_ms);
            }
            shim_relay_request_clear(&u->relay);
        }
        if (!u->relay.hello_ok && u->relay.req_state != SHIM_RELAY_REQ_PENDING &&
            (!u->hello_started || t - u->last_hello >= HELLO_RETRY_MS)) {
            shim_relay_hello_request(&u->relay, t);
            u->hello_started = 1;
            u->last_hello = t;
        }
    }
    u->ops->wake(u->ops->ctx);
    u->ops->unlock(u->ops->ctx);
}

void shim_uds_peek(shim_uds *u, shim_uds_status *st) {
    u->ops->lock(u->ops->ctx);
    *st = u->st;
    u->ops->unlock(u->ops->ctx);
}

int shim_uds_queued(shim_uds *u, uint32_t bind_id) {
    u->ops->lock(u->ops->ctx);
    const shim_uds_bind *b = find_bind(u, bind_id);
    const int n = b && in_network(u) ? (int)b->count : -1;
    u->ops->unlock(u->ops->ctx);
    return n;
}

int shim_uds_relay_up(shim_uds *u) {
    u->ops->lock(u->ops->ctx);
    const int up = u->relay.hello_ok;
    u->ops->unlock(u->ops->ctx);
    return up;
}

// ---- relay requests from session threads (lock held) ----------------------------------------

// Waits up to `ms` for the relay to answer HELLO.
static int relay_ready(shim_uds *u, uint32_t ms) {
    const uint64_t start = now(u);
    while (!u->relay.hello_ok) {
        if (now(u) - start >= ms) {
            return 0;
        }
        u->ops->wait(u->ops->ctx, 50);
    }
    return 1;
}

// Runs one relay request to completion, waiting for any request in flight first. Returns the
// response status (its payload in u->resp), or -1 if the relay did not answer. Runs with the lock held and returns with it held, so u->resp stays valid until the caller next
// waits.
static int relay_call(shim_uds *u, uint8_t type, const uint8_t *p, size_t n) {
    u->resp_len = 0;
    if (!relay_ready(u, HELLO_WAIT_MS)) {
        u->stats.relay_failures++;
        return -1;
    }
    const uint64_t start = now(u);
    while (u->busy || u->relay.req_state == SHIM_RELAY_REQ_PENDING) {
        if (now(u) - start >= SLOT_WAIT_MS) {
            u->stats.relay_failures++;
            return -1;
        }
        u->ops->wait(u->ops->ctx, 50);
    }
    u->busy = 1;
    if (type == SHIM_RELAY_SCAN_REQ) {
        u->nscan = 0;
    }
    int status = -1;
    const uint64_t sent = now(u);
    if (shim_relay_request(&u->relay, type, p, n, sent) == 0) {
        // The network thread times the request out; this deadline only covers that thread
        // being parked (HOME menu, sleep).
        while (u->relay.req_state == SHIM_RELAY_REQ_PENDING && now(u) - sent < u->relay.timeout_ms + 1000) {
            u->ops->wait(u->ops->ctx, 50);
        }
        if (u->relay.req_state == SHIM_RELAY_REQ_DONE) {
            status = u->relay.status;
            memcpy(u->resp, u->relay.resp, u->relay.resp_len);
            u->resp_len = u->relay.resp_len;
        }
        shim_relay_request_clear(&u->relay);
    }
    if (status < 0) {
        u->stats.relay_failures++;
    }
    u->busy = 0;
    u->ops->wake(u->ops->ctx);
    return status;
}

static void leave(shim_uds *u) {
    if (relay_call(u, SHIM_RELAY_LEAVE, NULL, 0) != SHIM_RELAY_STATUS_OK) {
        note(u, "Warning", "LEAVE not acknowledged; the relay will time us out");
    }
}

// ---- commands -------------------------------------------------------------------------------

static void reply(uint32_t *cmd, uint16_t id, unsigned normal, unsigned translate, uint32_t result) {
    cmd[0] = SHIM_IPC_MAKE_HEADER(id, normal, translate);
    cmd[1] = result;
}

// The n-th static buffer of a request (in descriptor order), or NULL.
static const uint8_t *static_buf(shim_uds *u, const shim_ipc_msg *m, int nth, uint32_t *size) {
    for (unsigned i = 0; i < m->ndesc; i++) {
        if (m->desc[i].kind != SHIM_DESC_STATIC_BUFFER) {
            continue;
        }
        if (nth-- == 0) {
            *size = m->desc[i].size;
            return u->ops->mem(u->ops->ctx, m->desc[i].ptr, m->desc[i].size);
        }
    }
    *size = 0;
    return NULL;
}

static void initialize(shim_uds *u, uint32_t *cmd) {
    uint8_t self[SHIM_RELAY_NODEINFO_SIZE];
    memcpy(self, &cmd[2], sizeof(self));
    if (!relay_ready(u, HELLO_WAIT_MS)) {
        note(u, "Error", "InitializeWithVersion: the relay does not answer; failing like NWM without local mode");
        reply(cmd, 0x001B, 1, 0, SHIM_UDS_RESULT_NO_RELAY);
        return;
    }
    if (u->status_event) {
        u->ops->close_handle(u->ops->ctx, u->status_event);
    }
    u->status_event = u->ops->create_event(u->ops->ctx);
    if (!u->status_event) {
        note(u, "Error", "InitializeWithVersion: could not create the status event");
        reply(cmd, 0x001B, 1, 0, SHIM_UDS_RESULT_OUT_OF_MEMORY);
        return;
    }
    memcpy(u->self, self, sizeof(self));
    u->inited = 1;
    drop_binds(u);
    const uint16_t node = u->st.node_id;
    memset(&u->st, 0, sizeof(u->st));
    u->st.status = SHIM_UDS_NOT_CONNECTED;
    u->st.node_id = node;
    memset(u->nodes, 0, sizeof(u->nodes));
    // 0x0023's word: low half 0x1fff, high half different every initialisation on hardware.
    const uint64_t t = now(u);
    u->word23 = (uint32_t)((t * 2654435761u) ^ (t >> 16) ^ u->relay.session) << 16 | 0x1fff;
    reply(cmd, 0x001B, 1, 2, 0);
    cmd[2] = 0; // copy one handle
    cmd[3] = u->status_event;
}

static void shutdown_uds(shim_uds *u, uint32_t *cmd) {
    if (in_network(u)) {
        leave(u);
    }
    const uint16_t node = u->st.node_id;
    memset(&u->st, 0, sizeof(u->st));
    u->st.status = SHIM_UDS_NOT_CONNECTED;
    u->st.node_id = node;
    drop_binds(u);
    if (u->status_event) {
        u->ops->close_handle(u->ops->ctx, u->status_event);
        u->status_event = 0;
    }
    u->inited = 0;
    reply(cmd, 0x0003, 1, 0, 0);
}

static void begin_hosting(shim_uds *u, uint32_t *cmd, const shim_ipc_msg *m) {
    uint32_t nsize, psize;
    const uint8_t *ni_in = static_buf(u, m, 0, &nsize);
    const uint8_t *pass = static_buf(u, m, 1, &psize);
    if (!ni_in || !pass || nsize < SHIM_RELAY_NETINFO_SIZE) {
        reply(cmd, 0x001D, 1, 0, SHIM_UDS_RESULT_CONNECT_FAILED);
        return;
    }
    if (cmd[1] < psize) {
        psize = cmd[1];
    }
    uint8_t ni[SHIM_RELAY_NETINFO_SIZE], hash[32];
    memcpy(ni, ni_in, sizeof(ni));
    shim_sha256(pass, psize, hash);
    if (in_network(u)) {
        leave(u);
    }
    memcpy(ni, u->mac, 6);
    ni[NI_CHANNEL] = SHIM_UDS_CHANNEL;
    memcpy(ni + NI_OUI, kOUI, 3);
    ni[NI_OUI_TYPE] = TAG_NETINFO;
    uint8_t req[512];
    const size_t n = shim_relay_host_create(req, sizeof(req), hash, u->self, ni);
    if (relay_call(u, SHIM_RELAY_HOST_CREATE, req, n) != SHIM_RELAY_STATUS_OK || u->resp_len < 4) {
        note(u, "Warning", "BeginHostingNetwork: the relay refused or did not answer");
        reply(cmd, 0x001D, 1, 0, SHIM_UDS_RESULT_CONNECT_FAILED);
        return;
    }
    const uint32_t id = get32be(u->resp);
    put32be(ni + NI_NETWORK_ID, id);
    ni[NI_TOTAL] = 1;
    memcpy(u->netinfo, ni, sizeof(ni));
    memset(u->nodes, 0, sizeof(u->nodes));
    memcpy(u->nodes[0], u->self, SHIM_RELAY_NODEINFO_SIZE);
    memset(&u->st, 0, sizeof(u->st));
    u->st.status = SHIM_UDS_HOST;
    u->st.reason = SHIM_UDS_REASON_ESTABLISHED;
    u->st.node_id = 1;
    u->st.changed = 1;
    u->st.total = 1;
    u->st.max = (uint8_t)max_nodes(ni);
    u->st.bitmask = 1;
    note(u, "Info", "hosting network 0x%08lx (comm ID 0x%08lx, ID8 %u, %u nodes)", (unsigned long)id,
         (unsigned long)get32be(ni + 0x10), (unsigned)ni[0x14], (unsigned)u->st.max);
    signal_status(u);
    reply(cmd, 0x001D, 1, 0, 0);
}

static void destroy_network(shim_uds *u, uint32_t *cmd) {
    if (u->st.status != SHIM_UDS_HOST) {
        reply(cmd, 0x0008, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    leave(u);
    not_connected(u, SHIM_UDS_REASON_LEFT);
    reply(cmd, 0x0008, 1, 0, 0);
}

static void set_app_data(shim_uds *u, uint32_t *cmd, const shim_ipc_msg *m) {
    uint32_t size;
    const uint8_t *data = static_buf(u, m, 0, &size);
    if (cmd[1] < size) {
        size = cmd[1];
    }
    if (u->st.status != SHIM_UDS_HOST || !data || size > MAX_APPDATA) {
        reply(cmd, 0x0010, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    uint8_t copy[MAX_APPDATA], req[2 + MAX_APPDATA];
    memcpy(copy, data, size);
    const size_t n = shim_relay_update_appdata(req, sizeof(req), copy, size);
    if (relay_call(u, SHIM_RELAY_HOST_UPDATE, req, n) != SHIM_RELAY_STATUS_OK) {
        reply(cmd, 0x0010, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    memset(u->netinfo + NI_APPDATA, 0, MAX_APPDATA);
    memcpy(u->netinfo + NI_APPDATA, copy, size);
    u->netinfo[NI_APPDATA_SIZE] = (uint8_t)size;
    reply(cmd, 0x0010, 1, 0, 0);
}

static void get_app_data(shim_uds *u, uint32_t *cmd, uint8_t *out) {
    if (!in_network(u)) {
        reply(cmd, 0x0011, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    uint32_t size = u->netinfo[NI_APPDATA_SIZE];
    if (size > MAX_APPDATA) {
        size = MAX_APPDATA;
    }
    if (cmd[1] < size) {
        size = 0;
    }
    memcpy(out, u->netinfo + NI_APPDATA, size);
    reply(cmd, 0x0011, 2, 2, 0);
    cmd[2] = size;
    cmd[3] = SHIM_IPC_STATIC_DESC(size, 0);
    cmd[4] = u->ops->addr_of(u->ops->ctx, out);
}

static void recv_beacons(shim_uds *u, uint32_t *cmd, const shim_ipc_msg *m) {
    const uint32_t out_size = cmd[1];
    const uint32_t comm_id = cmd[15];
    const uint8_t id8 = (uint8_t)cmd[16];
    uint32_t desc = 0, ptr = 0, size = 0;
    for (unsigned i = 0; i < m->ndesc; i++) {
        if (m->desc[i].kind == SHIM_DESC_MAPPED_BUFFER) {
            desc = cmd[m->desc[i].word];
            ptr = m->desc[i].ptr;
            size = m->desc[i].size;
        }
    }
    if (out_size < size) {
        size = out_size;
    }
    uint8_t req[8];
    const size_t n = shim_relay_scan(req, sizeof(req), comm_id, id8);
    if (relay_call(u, SHIM_RELAY_SCAN_REQ, req, n) != SHIM_RELAY_STATUS_OK) {
        note(u, "Warning", "scan for 0x%08lx/%u failed", (unsigned long)comm_id, (unsigned)id8);
        u->nscan = 0;
    }
    uint8_t *buf = size >= 12 ? u->ops->mem(u->ops->ctx, ptr, size) : NULL;
    if (buf) {
        size_t used = 12;
        uint32_t entries = 0;
        for (int i = 0; i < u->nscan; i++) {
            shim_uds_network net;
            if (shim_uds_parse_network(u->scan[i], u->scan_len[i], &net) != 0) {
                continue;
            }
            const size_t e = shim_uds_beacon_entry(buf + used, size - used, &net);
            if (e) {
                used += e;
                entries++;
            }
        }
        put32le(buf, out_size);
        put32le(buf + 4, (uint32_t)used);
        put32le(buf + 8, entries);
    }
    reply(cmd, 0x000F, 1, 2, 0);
    cmd[2] = desc;
    cmd[3] = ptr;
}

static void decrypt_beacon(shim_uds *u, uint32_t *cmd, const shim_ipc_msg *m, uint8_t *out) {
    uint32_t nn, n0, n1;
    const uint8_t *ni = static_buf(u, m, 0, &nn);
    const uint8_t *t0 = static_buf(u, m, 1, &n0);
    const uint8_t *t1 = static_buf(u, m, 2, &n1);
    const size_t outn = SHIM_RELAY_MAX_NODES * SHIM_RELAY_NODEINFO_SIZE;
    if (ni && nn >= SHIM_RELAY_NETINFO_SIZE) {
        shim_uds_decrypt_nodes(ni, t0, t0 ? n0 : 0, t1, t1 ? n1 : 0, out);
    } else {
        memset(out, 0, outn);
    }
    reply(cmd, 0x001F, 1, 2, 0);
    cmd[2] = SHIM_IPC_STATIC_DESC(outn, 0);
    cmd[3] = u->ops->addr_of(u->ops->ctx, out);
}

static void connect_network(shim_uds *u, uint32_t *cmd, const shim_ipc_msg *m) {
    const uint8_t conn_type = (uint8_t)cmd[1];
    uint32_t nsize, psize;
    const uint8_t *ni = static_buf(u, m, 0, &nsize);
    const uint8_t *pass = static_buf(u, m, 1, &psize);
    if (!ni || !pass || nsize < 0x1C) {
        reply(cmd, 0x001E, 1, 0, SHIM_UDS_RESULT_CONNECT_FAILED);
        return;
    }
    if (cmd[2] < psize) {
        psize = cmd[2];
    }
    uint8_t hash[32];
    shim_sha256(pass, psize, hash);
    const uint32_t id = get32be(ni + NI_NETWORK_ID);
    if (in_network(u)) {
        leave(u);
    }
    uint8_t req[128];
    const uint8_t ct = conn_type == SHIM_RELAY_CONN_SPECTATOR ? SHIM_RELAY_CONN_SPECTATOR : SHIM_RELAY_CONN_CLIENT;
    const size_t n = shim_relay_join(req, sizeof(req), id, hash, ct, u->self);
    const int status = relay_call(u, SHIM_RELAY_JOIN_REQ, req, n);
    shim_uds_network net;
    if (status != SHIM_RELAY_STATUS_OK || u->resp_len < 2 || shim_uds_parse_network(u->resp + 2, u->resp_len - 2, &net) != 0) {
        note(u, "Warning", "ConnectToNetwork 0x%08lx: relay status %d", (unsigned long)id, status);
        reply(cmd, 0x001E, 1, 0, SHIM_UDS_RESULT_CONNECT_FAILED);
        return;
    }
    memcpy(u->netinfo, net.netinfo, SHIM_RELAY_NETINFO_SIZE);
    memset(u->nodes, 0, sizeof(u->nodes));
    for (int i = 0; i < SHIM_RELAY_MAX_NODES; i++) {
        if (net.nodes[i]) {
            memcpy(u->nodes[i], net.nodes[i], SHIM_RELAY_NODEINFO_SIZE);
        }
    }
    memset(&u->st, 0, sizeof(u->st));
    u->st.status = ct == SHIM_RELAY_CONN_SPECTATOR ? SHIM_UDS_SPECTATOR : SHIM_UDS_CLIENT;
    u->st.reason = SHIM_UDS_REASON_ESTABLISHED;
    u->st.node_id = get16be(u->resp);
    u->st.changed = net.bitmask;
    u->st.total = net.netinfo[NI_TOTAL];
    u->st.max = net.netinfo[NI_MAX];
    u->st.bitmask = net.bitmask;
    note(u, "Info", "joined network 0x%08lx as node %u", (unsigned long)id, (unsigned)u->st.node_id);
    signal_status(u);
    reply(cmd, 0x001E, 1, 0, 0);
}

static void disconnect_network(shim_uds *u, uint32_t *cmd) {
    if (u->st.status != SHIM_UDS_CLIENT && u->st.status != SHIM_UDS_SPECTATOR) {
        reply(cmd, 0x000A, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    leave(u);
    not_connected(u, SHIM_UDS_REASON_LEFT);
    reply(cmd, 0x000A, 1, 0, 0);
}

static void eject(shim_uds *u, uint32_t *cmd, uint16_t id, uint16_t node) {
    uint8_t req[4];
    const size_t n = shim_relay_eject(req, sizeof(req), node);
    if (u->st.status != SHIM_UDS_HOST || relay_call(u, SHIM_RELAY_EJECT, req, n) != SHIM_RELAY_STATUS_OK) {
        reply(cmd, id, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    reply(cmd, id, 1, 0, 0);
}

static void connection_status(shim_uds *u, uint32_t *cmd) {
    uint8_t b[0x30];
    memset(b, 0, sizeof(b));
    put32le(b, u->st.status);
    put32le(b + 4, u->st.reason);
    put16le(b + 8, u->st.node_id);
    put16le(b + 0xA, u->st.changed);
    for (int i = 0; i < SHIM_RELAY_MAX_NODES; i++) {
        if (u->st.bitmask & (1u << i)) {
            put16le(b + 0xC + 2 * i, (uint16_t)(i + 1));
        }
    }
    b[0x2C] = u->st.total;
    b[0x2D] = u->st.max;
    put16le(b + 0x2E, u->st.bitmask);
    u->st.changed = 0;
    reply(cmd, 0x000B, 13, 0, 0);
    memcpy(&cmd[2], b, sizeof(b));
}

static void node_information(shim_uds *u, uint32_t *cmd) {
    const uint16_t node = (uint16_t)cmd[1];
    if (!in_network(u) || node == 0 || node > SHIM_RELAY_MAX_NODES || !(u->st.bitmask & (1u << (node - 1)))) {
        reply(cmd, 0x000D, 1, 0, SHIM_UDS_RESULT_NOT_FOUND);
        return;
    }
    uint8_t info[SHIM_RELAY_NODEINFO_SIZE];
    memcpy(info, u->nodes[node - 1], sizeof(info));
    put16le(info + NODE_ID_OFFSET, node);
    reply(cmd, 0x000D, 11, 0, 0);
    memcpy(&cmd[2], info, sizeof(info));
}

static void bind(shim_uds *u, uint32_t *cmd) {
    const uint32_t id = cmd[1];
    shim_uds_bind *b = find_bind(u, id);
    if (b) {
        drop_bind(u, b);
    } else {
        for (int i = 0; i < SHIM_UDS_MAX_BINDS && !b; i++) {
            if (!u->binds[i].used) {
                b = &u->binds[i];
            }
        }
    }
    shim_handle_t ev = b ? u->ops->create_event(u->ops->ctx) : 0;
    if (!ev) {
        reply(cmd, 0x0012, 1, 0, SHIM_UDS_RESULT_OUT_OF_MEMORY);
        return;
    }
    b->used = 1;
    b->id = id;
    b->buf_size = cmd[2] ? cmd[2] : DEFAULT_BIND_BUF;
    b->channel = (uint8_t)cmd[3];
    b->node = (uint16_t)cmd[4];
    b->head = b->tail = NONE;
    b->event = ev;
    reply(cmd, 0x0012, 1, 2, 0);
    cmd[2] = 0;
    cmd[3] = ev;
}

static void unbind(shim_uds *u, uint32_t *cmd) {
    const uint32_t id = cmd[1];
    shim_uds_bind *b = find_bind(u, id);
    uint32_t w2 = 0, w3 = 0;
    if (b) {
        w2 = (uint32_t)b->channel << 16 | b->node;
        w3 = b->received;
        drop_bind(u, b);
    }
    reply(cmd, 0x0013, 5, 0, 0);
    cmd[2] = id;
    cmd[3] = w2;
    cmd[4] = w3;
    cmd[5] = 0;
}

static void send_to(shim_uds *u, uint32_t *cmd, const shim_ipc_msg *m) {
    const uint16_t dst = (uint16_t)cmd[2];
    const uint8_t channel = (uint8_t)cmd[3];
    const uint8_t flags = (uint8_t)cmd[6];
    uint32_t size;
    const uint8_t *data = static_buf(u, m, 0, &size);
    if (cmd[5] < size) {
        size = cmd[5];
    }
    if (!in_network(u) || u->st.status == SHIM_UDS_SPECTATOR || !data) {
        reply(cmd, 0x0017, 1, 0, SHIM_UDS_RESULT_INVALID_STATE);
        return;
    }
    if (shim_relay_send_data(&u->relay, dst, channel, flags, data, size) != 0) {
        note(u, "Warning", "SendTo: %lu-byte frame does not fit a datagram", (unsigned long)size);
    }
    reply(cmd, 0x0017, 1, 0, 0);
}

static void pull_packet(shim_uds *u, uint32_t *cmd, uint8_t *out) {
    const uint32_t id = cmd[1];
    const uint32_t max = cmd[3];
    uint32_t result = 0, size = 0, src = 0;
    shim_uds_bind *b = find_bind(u, id);
    if (!in_network(u) || !b) {
        result = SHIM_UDS_RESULT_INVALID_STATE;
    } else if (b->head != NONE) {
        shim_uds_packet *pk = &u->packets[b->head];
        if (max && pk->len > max) {
            result = SHIM_UDS_RESULT_INVALID_STATE; // what NWM does here is untested; keep the frame
        } else {
            size = pk->len;
            src = pk->src;
            memcpy(out, pk->data, size);
            const uint16_t s = b->head;
            b->head = pk->next;
            if (b->head == NONE) {
                b->tail = NONE;
            }
            pk->next = u->free_head;
            u->free_head = s;
            b->count--;
            b->queued_bytes -= size;
        }
    }
    // Hardware always replies in this shape, describing a 0x5C8-byte buffer, even for errors.
    reply(cmd, 0x0014, 3, 2, result);
    cmd[2] = size;
    cmd[3] = src;
    cmd[4] = SHIM_IPC_STATIC_DESC(SHIM_UDS_MAX_FRAME, 0);
    cmd[5] = u->ops->addr_of(u->ops->ctx, out);
}

void shim_uds_handle(shim_uds *u, uint32_t *cmd, uint8_t *out) {
    shim_ipc_msg m;
    if (shim_ipc_parse(cmd, &m) != 0) {
        m.ndesc = 0;
    }
    const uint16_t id = m.cmd;
    u->ops->lock(u->ops->ctx);
    switch (id) {
    case 0x001B:
        initialize(u, cmd);
        break;
    case 0x0003:
        shutdown_uds(u, cmd);
        break;
    case 0x001D:
        begin_hosting(u, cmd, &m);
        break;
    case 0x0008:
        destroy_network(u, cmd);
        break;
    case 0x0010:
        set_app_data(u, cmd, &m);
        break;
    case 0x0011:
        get_app_data(u, cmd, out);
        break;
    case 0x000F:
        recv_beacons(u, cmd, &m);
        break;
    case 0x001F:
        decrypt_beacon(u, cmd, &m, out);
        break;
    case 0x001E:
        connect_network(u, cmd, &m);
        break;
    case 0x000A:
        disconnect_network(u, cmd);
        break;
    case 0x0005:
        eject(u, cmd, id, (uint16_t)cmd[1]);
        break;
    case 0x0006:
        eject(u, cmd, id, 0);
        break;
    case 0x000B:
        connection_status(u, cmd);
        break;
    case 0x000D:
        node_information(u, cmd);
        break;
    case 0x0012:
        bind(u, cmd);
        break;
    case 0x0013:
        unbind(u, cmd);
        break;
    case 0x0017:
        send_to(u, cmd, &m);
        break;
    case 0x0014:
        pull_packet(u, cmd, out);
        break;
    case 0x001A: {
        const uint32_t ch = in_network(u) ? SHIM_UDS_CHANNEL : 0;
        reply(cmd, 0x001A, 2, 0, 0);
        cmd[2] = ch;
        break;
    }
    case 0x0023: // unnamed; hardware answers success and one word
        reply(cmd, 0x0023, 2, 0, 0);
        cmd[2] = u->word23;
        break;
    case 0x0007: // UpdateNetworkAttribute
    case 0x0015: // SetMaxSendDelay
    case 0x0020: // Flush
    case 0x0021: // SetProbeResponseParam: success on hardware
    case 0x0022: // ScanOnConnection
        reply(cmd, id, 1, 0, 0);
        break;
    default:
        // Unknown: succeed with no output and say so, so gaps surface quickly.
        u->stats.unknown++;
        note(u, "Warning", "nwm::UDS command 0x%04x not emulated (header 0x%08lx); answered success", (unsigned)id,
             (unsigned long)cmd[0]);
        reply(cmd, id, 1, 0, 0);
        break;
    }
    u->ops->unlock(u->ops->ctx);
}
