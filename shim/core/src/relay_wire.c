#include "shim/relay_wire.h"

#include <string.h>

#include "shim/sha256.h"

static void put16(uint8_t *p, uint16_t v) {
    p[0] = (uint8_t)(v >> 8);
    p[1] = (uint8_t)v;
}

static void put32(uint8_t *p, uint32_t v) {
    p[0] = (uint8_t)(v >> 24);
    p[1] = (uint8_t)(v >> 16);
    p[2] = (uint8_t)(v >> 8);
    p[3] = (uint8_t)v;
}

static uint32_t get32(const uint8_t *p) {
    return (uint32_t)p[0] << 24 | (uint32_t)p[1] << 16 | (uint32_t)p[2] << 8 | p[3];
}

void shim_relay_header_put(uint8_t *out, const shim_relay_header *h) {
    out[0] = 'U';
    out[1] = 'R';
    out[2] = SHIM_RELAY_VERSION;
    out[3] = h->type;
    put32(out + 4, h->session);
    put16(out + 8, h->seq);
    put16(out + 10, h->ack);
}

int shim_relay_header_get(const uint8_t *b, size_t len, shim_relay_header *h) {
    if (len < SHIM_RELAY_HEADER_SIZE || b[0] != 'U' || b[1] != 'R' || b[2] != SHIM_RELAY_VERSION) {
        return -1;
    }
    h->type = b[3];
    h->session = get32(b + 4);
    h->seq = (uint16_t)(b[8] << 8 | b[9]);
    h->ack = (uint16_t)(b[10] << 8 | b[11]);
    return 0;
}

uint32_t shim_relay_lobby_id(const char *key, size_t key_len) {
    static const char prefix[] = "udsrelay lobby id"; // plus its NUL, as in the Go side
    uint8_t sum[SHIM_SHA256_SIZE];
    shim_sha256_ctx s;
    shim_sha256_init(&s);
    shim_sha256_update(&s, prefix, sizeof(prefix));
    shim_sha256_update(&s, key, key_len);
    shim_sha256_final(&s, sum);
    return get32(sum);
}

size_t shim_relay_seal(const void *key, size_t key_len, uint8_t *buf, size_t len, size_t cap) {
    if (len + SHIM_RELAY_TAG_SIZE > cap) {
        return 0;
    }
    uint8_t mac[SHIM_SHA256_SIZE];
    shim_hmac_sha256(key, key_len, buf, len, mac);
    memcpy(buf + len, mac, SHIM_RELAY_TAG_SIZE);
    return len + SHIM_RELAY_TAG_SIZE;
}

long shim_relay_open(const void *key, size_t key_len, const uint8_t *buf, size_t len) {
    if (len < SHIM_RELAY_HEADER_SIZE + SHIM_RELAY_TAG_SIZE) {
        return -1;
    }
    const size_t body = len - SHIM_RELAY_TAG_SIZE;
    uint8_t mac[SHIM_SHA256_SIZE];
    shim_hmac_sha256(key, key_len, buf, body, mac);
    uint8_t diff = 0;
    for (int i = 0; i < SHIM_RELAY_TAG_SIZE; i++) {
        diff |= mac[i] ^ buf[body + i];
    }
    return diff ? -1 : (long)body;
}

size_t shim_relay_hello(const char *key, size_t key_len, uint32_t session, uint16_t seq, uint8_t *out,
                        size_t cap) {
    if (cap < SHIM_RELAY_HEADER_SIZE + 4) {
        return 0;
    }
    const shim_relay_header h = {SHIM_RELAY_HELLO, session, seq, 0};
    shim_relay_header_put(out, &h);
    put32(out + SHIM_RELAY_HEADER_SIZE, shim_relay_lobby_id(key, key_len));
    return shim_relay_seal(key, key_len, out, SHIM_RELAY_HEADER_SIZE + 4, cap);
}
