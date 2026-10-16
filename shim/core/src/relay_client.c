#include "shim/relay_client.h"

#include <string.h>

#define DEFAULT_RETRY_MS 250
#define DEFAULT_TIMEOUT_MS 3000
#define DEFAULT_KEEPALIVE_MS 1000

static void put16(uint8_t *p, uint16_t v) {
    p[0] = (uint8_t)(v >> 8);
    p[1] = (uint8_t)v;
}

static void put32(uint8_t *p, uint32_t v) {
    put16(p, (uint16_t)(v >> 16));
    put16(p + 2, (uint16_t)v);
}

static uint16_t get16(const uint8_t *p) { return (uint16_t)(p[0] << 8 | p[1]); }

static uint32_t get32(const uint8_t *p) { return (uint32_t)get16(p) << 16 | get16(p + 2); }

// seq_after reports whether a comes after b in uint16 serial-number order.
static int seq_after(uint16_t a, uint16_t b) { return (int16_t)(uint16_t)(a - b) > 0; }

static uint16_t next_seq(shim_relay_client *c) {
    if (++c->seq == 0) {
        c->seq = 1;
    }
    return c->seq;
}

// Builds and sends one sealed datagram. Returns its sealed length (0 if it did not fit).
static size_t emit(shim_relay_client *c, uint8_t *buf, uint8_t type, uint16_t seq, uint16_t ack, const uint8_t *payload,
                   size_t n) {
    if (SHIM_RELAY_HEADER_SIZE + n + SHIM_RELAY_TAG_SIZE > SHIM_RELAY_MAX_DATAGRAM) {
        return 0;
    }
    const shim_relay_header h = {type, c->session, seq, ack};
    shim_relay_header_put(buf, &h);
    if (n) {
        memcpy(buf + SHIM_RELAY_HEADER_SIZE, payload, n);
    }
    const size_t len = shim_relay_seal(c->key, c->key_len, buf, SHIM_RELAY_HEADER_SIZE + n, SHIM_RELAY_MAX_DATAGRAM);
    if (len) {
        c->ops->send(c->ops->ctx, buf, len);
    }
    return len;
}

int shim_relay_client_init(shim_relay_client *c, const shim_relay_ops *ops, const char *key, size_t key_len,
                           uint32_t session) {
    if (!key_len || key_len > SHIM_RELAY_MAX_LOBBY_KEY || !session) {
        return -1;
    }
    memset(c, 0, sizeof(*c));
    c->ops = ops;
    memcpy(c->key, key, key_len);
    c->key_len = key_len;
    c->session = session;
    c->retry_ms = DEFAULT_RETRY_MS;
    c->timeout_ms = DEFAULT_TIMEOUT_MS;
    c->keepalive_ms = DEFAULT_KEEPALIVE_MS;
    return 0;
}

int shim_relay_request(shim_relay_client *c, uint8_t type, const uint8_t *payload, size_t n, uint64_t now) {
    if (c->req_state == SHIM_RELAY_REQ_PENDING) {
        return -1;
    }
    c->req_type = type;
    c->req_seq = next_seq(c);
    c->req_len = emit(c, c->req, type, c->req_seq, 0, payload, n);
    if (!c->req_len) {
        c->req_state = SHIM_RELAY_REQ_IDLE;
        return -1;
    }
    c->req_state = SHIM_RELAY_REQ_PENDING;
    c->req_start = c->req_sent = now;
    c->status = 0;
    c->resp_len = 0;
    c->scan_got = 0;
    c->scan_count = c->scan_total = 0;
    return 0;
}

int shim_relay_hello_request(shim_relay_client *c, uint64_t now) {
    uint8_t p[4];
    put32(p, shim_relay_lobby_id(c->key, c->key_len));
    if (shim_relay_request(c, SHIM_RELAY_HELLO, p, sizeof(p), now) != 0) {
        return -1;
    }
    c->need_hello = 0;
    return 0;
}

void shim_relay_request_clear(shim_relay_client *c) {
    if (c->req_state != SHIM_RELAY_REQ_PENDING) {
        c->req_state = SHIM_RELAY_REQ_IDLE;
    }
}

static void finish(shim_relay_client *c, uint8_t status, const uint8_t *p, size_t n) {
    c->status = status;
    c->resp_len = n < sizeof(c->resp) ? n : sizeof(c->resp);
    if (c->resp_len) {
        memcpy(c->resp, p, c->resp_len);
    }
    c->req_state = SHIM_RELAY_REQ_DONE;
}

// A response to the request in flight: p is the payload, starting with the status byte.
static void response(shim_relay_client *c, uint8_t type, const uint8_t *p, size_t n) {
    if (type != (uint8_t)(c->req_type + 1) || n < 1) { // every response type is its request's + 1
        return;
    }
    const uint8_t status = p[0];
    if (type == SHIM_RELAY_SCAN_RESP && status == SHIM_RELAY_STATUS_OK) {
        if (n < 3) {
            return;
        }
        const uint8_t index = p[1], total = p[2];
        if (total == 0) {
            finish(c, status, NULL, 0);
            return;
        }
        const uint32_t bit = index < 32 ? 1u << index : 0;
        if (bit && (c->scan_got & bit)) {
            return; // a retry's duplicate
        }
        c->scan_got |= bit;
        c->scan_count++;
        c->scan_total = total;
        if (c->ops->on_scan) {
            c->ops->on_scan(c->ops->ctx, index, total, p + 3, n - 3);
        }
        if (c->scan_count >= total) {
            finish(c, status, NULL, 0);
        }
        return;
    }
    if (type == SHIM_RELAY_HELLO_ACK && status == SHIM_RELAY_STATUS_OK && n >= 3) {
        const uint16_t ka = get16(p + 1);
        c->keepalive_ms = ka ? ka : DEFAULT_KEEPALIVE_MS;
        c->hello_ok = 1;
    }
    finish(c, status, p + 1, n - 1);
}

static void node_event(shim_relay_client *c, const shim_relay_header *h, const uint8_t *p, size_t n, uint64_t now) {
    uint8_t ack[SHIM_RELAY_HEADER_SIZE + SHIM_RELAY_TAG_SIZE];
    emit(c, ack, SHIM_RELAY_ACK, next_seq(c), h->seq, NULL, 0);
    (void)now;
    if (c->ev_any && !seq_after(h->seq, c->ev_last)) {
        return; // a retransmission we already delivered
    }
    if (n < 13) {
        return;
    }
    c->ev_any = 1;
    c->ev_last = h->seq;
    shim_relay_event e;
    memset(&e, 0, sizeof(e));
    e.kind = p[0];
    e.reason = p[1];
    e.network_id = get32(p + 2);
    e.node = get16(p + 6);
    e.bitmask = get16(p + 8);
    e.total = p[10];
    if (n >= 11 + SHIM_RELAY_NODEINFO_SIZE) {
        memcpy(e.info, p + 11, SHIM_RELAY_NODEINFO_SIZE);
    }
    c->stats.events++;
    if (c->ops->on_event) {
        c->ops->on_event(c->ops->ctx, &e);
    }
}

void shim_relay_input(shim_relay_client *c, const uint8_t *b, size_t n, uint64_t now) {
    const long body = shim_relay_open(c->key, c->key_len, b, n);
    shim_relay_header h;
    if (body < 0 || shim_relay_header_get(b, (size_t)body, &h) != 0 || h.session != c->session) {
        c->stats.bad_tag++;
        return;
    }
    const uint8_t *p = b + SHIM_RELAY_HEADER_SIZE;
    const size_t pn = (size_t)body - SHIM_RELAY_HEADER_SIZE;
    // The server numbers data, replies and keepalive answers as one stream per session (events
    // have their own), so a gap there is a datagram the network lost. Resent replies repeat an
    // old number and are not counted.
    if (h.type != SHIM_RELAY_NODE_EVENT) {
        if (c->in_any && seq_after(h.seq, c->in_last)) {
            c->stats.lost += (uint16_t)(h.seq - c->in_last - 1);
        }
        if (!c->in_any || seq_after(h.seq, c->in_last)) {
            c->in_last = h.seq;
            c->in_any = 1;
        }
    }
    switch (h.type) {
    case SHIM_RELAY_DATA:
        if (pn >= 6 && c->ops->on_data) {
            c->stats.data_in++;
            c->ops->on_data(c->ops->ctx, get16(p), get16(p + 2), p[4], p[5], p + 6, pn - 6);
        }
        break;
    case SHIM_RELAY_NODE_EVENT:
        node_event(c, &h, p, pn, now);
        break;
    case SHIM_RELAY_KEEPALIVE_ACK:
        if (pn >= 8) {
            const uint64_t sent = (uint64_t)get32(p) << 32 | get32(p + 4);
            c->rtt_ms = (uint32_t)(now - sent);
        }
        break;
    case SHIM_RELAY_RESET:
        c->stats.resets++;
        c->ev_any = 0;
        c->hello_ok = 0;
        c->need_hello = 1;
        if (c->req_state == SHIM_RELAY_REQ_PENDING && c->req_type != SHIM_RELAY_HELLO) {
            c->req_state = SHIM_RELAY_REQ_FAILED; // the relay no longer knows us; don't wait it out
        }
        if (c->ops->on_reset) {
            c->ops->on_reset(c->ops->ctx);
        }
        break;
    default:
        if (c->req_state == SHIM_RELAY_REQ_PENDING && h.ack == c->req_seq) {
            response(c, h.type, p, pn);
        }
        break;
    }
}

void shim_relay_poll(shim_relay_client *c, uint64_t now) {
    if (c->req_state == SHIM_RELAY_REQ_PENDING) {
        if (now - c->req_start >= c->timeout_ms) {
            if (c->req_type == SHIM_RELAY_SCAN_REQ && c->scan_count) {
                finish(c, SHIM_RELAY_STATUS_OK, NULL, 0); // a lost SCAN_RESP just drops that network
            } else {
                c->req_state = SHIM_RELAY_REQ_FAILED;
            }
        } else if (now - c->req_sent >= c->retry_ms) {
            c->ops->send(c->ops->ctx, c->req, c->req_len);
            c->req_sent = now;
            c->stats.retries++;
        }
    }
    if (c->need_hello && c->req_state != SHIM_RELAY_REQ_PENDING) {
        shim_relay_hello_request(c, now);
    }
    if (c->hello_ok && now - c->last_keepalive >= c->keepalive_ms) {
        uint8_t p[8], buf[SHIM_RELAY_HEADER_SIZE + 8 + SHIM_RELAY_TAG_SIZE];
        put32(p, (uint32_t)(now >> 32));
        put32(p + 4, (uint32_t)now);
        emit(c, buf, SHIM_RELAY_KEEPALIVE, next_seq(c), 0, p, sizeof(p));
        c->last_keepalive = now;
    }
}

int shim_relay_send_data(shim_relay_client *c, uint16_t dst, uint8_t channel, uint8_t flags, const uint8_t *payload,
                         size_t n) {
    uint8_t buf[SHIM_RELAY_MAX_DATAGRAM];
    uint8_t *p = buf + SHIM_RELAY_HEADER_SIZE;
    if (SHIM_RELAY_HEADER_SIZE + 6 + n + SHIM_RELAY_TAG_SIZE > sizeof(buf)) {
        return -1;
    }
    put16(p, 0); // the relay fills in our node ID
    put16(p + 2, dst);
    p[4] = channel;
    p[5] = flags;
    memcpy(p + 6, payload, n);
    const shim_relay_header h = {SHIM_RELAY_DATA, c->session, next_seq(c), 0};
    shim_relay_header_put(buf, &h);
    const size_t len = shim_relay_seal(c->key, c->key_len, buf, SHIM_RELAY_HEADER_SIZE + 6 + n, sizeof(buf));
    c->ops->send(c->ops->ctx, buf, len);
    c->stats.data_out++;
    return 0;
}

size_t shim_relay_host_create(uint8_t *out, size_t cap, const uint8_t pass_hash[32],
                              const uint8_t self[SHIM_RELAY_NODEINFO_SIZE], const uint8_t netinfo[SHIM_RELAY_NETINFO_SIZE]) {
    const size_t n = 32 + SHIM_RELAY_NODEINFO_SIZE + SHIM_RELAY_NETINFO_SIZE;
    if (cap < n) {
        return 0;
    }
    memcpy(out, pass_hash, 32);
    memcpy(out + 32, self, SHIM_RELAY_NODEINFO_SIZE);
    memcpy(out + 32 + SHIM_RELAY_NODEINFO_SIZE, netinfo, SHIM_RELAY_NETINFO_SIZE);
    return n;
}

size_t shim_relay_update_appdata(uint8_t *out, size_t cap, const uint8_t *data, size_t n) {
    if (n > 0xC8 || cap < 2 + n) {
        return 0;
    }
    out[0] = SHIM_RELAY_UPDATE_APPDATA;
    out[1] = (uint8_t)n;
    memcpy(out + 2, data, n);
    return 2 + n;
}

size_t shim_relay_update_attributes(uint8_t *out, size_t cap, uint16_t attributes) {
    if (cap < 3) {
        return 0;
    }
    out[0] = SHIM_RELAY_UPDATE_ATTRIBUTES;
    put16(out + 1, attributes);
    return 3;
}

size_t shim_relay_scan(uint8_t *out, size_t cap, uint32_t comm_id, uint8_t id8) {
    if (cap < 5) {
        return 0;
    }
    put32(out, comm_id);
    out[4] = id8;
    return 5;
}

size_t shim_relay_join(uint8_t *out, size_t cap, uint32_t network_id, const uint8_t pass_hash[32], uint8_t conn_type,
                       const uint8_t self[SHIM_RELAY_NODEINFO_SIZE]) {
    const size_t n = 4 + 32 + 1 + SHIM_RELAY_NODEINFO_SIZE;
    if (cap < n) {
        return 0;
    }
    put32(out, network_id);
    memcpy(out + 4, pass_hash, 32);
    out[36] = conn_type;
    memcpy(out + 37, self, SHIM_RELAY_NODEINFO_SIZE);
    return n;
}

size_t shim_relay_eject(uint8_t *out, size_t cap, uint16_t node) {
    if (cap < 2) {
        return 0;
    }
    put16(out, node);
    return 2;
}
