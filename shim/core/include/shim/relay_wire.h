// Relay wire protocol v2, the console side of internal/relay/wire.go (see relay/RUNBOOK.md).
//
// Every datagram is a 12-byte big-endian header, a payload, and a 16-byte tag: the first 16 bytes
// of HMAC-SHA256 over header and payload, keyed by the lobby key. The key is never sent; HELLO
// names the lobby by shim_relay_lobby_id.
#pragma once

#include <stddef.h>
#include <stdint.h>

#define SHIM_RELAY_VERSION 2
#define SHIM_RELAY_HEADER_SIZE 12
#define SHIM_RELAY_TAG_SIZE 16
#define SHIM_RELAY_MAX_LOBBY_KEY 64
#define SHIM_RELAY_MAX_DATAGRAM 2048
#define SHIM_RELAY_BROADCAST 0xFFFF

enum {
    SHIM_RELAY_HELLO = 1,
    SHIM_RELAY_HELLO_ACK = 2,
    SHIM_RELAY_HOST_CREATE = 3,
    SHIM_RELAY_HOST_ACK = 4,
    SHIM_RELAY_HOST_UPDATE = 5,
    SHIM_RELAY_UPDATE_ACK = 6,
    SHIM_RELAY_SCAN_REQ = 7,
    SHIM_RELAY_SCAN_RESP = 8,
    SHIM_RELAY_JOIN_REQ = 9,
    SHIM_RELAY_JOIN_ACK = 10,
    SHIM_RELAY_NODE_EVENT = 11,
    SHIM_RELAY_DATA = 12,
    SHIM_RELAY_EJECT = 13,
    SHIM_RELAY_EJECT_ACK = 14,
    SHIM_RELAY_LEAVE = 15,
    SHIM_RELAY_LEAVE_ACK = 16,
    SHIM_RELAY_KEEPALIVE = 17,
    SHIM_RELAY_KEEPALIVE_ACK = 18,
    SHIM_RELAY_ACK = 19,
    SHIM_RELAY_RESET = 20,
};

typedef struct {
    uint8_t type;
    uint32_t session;
    uint16_t seq;
    uint16_t ack;
} shim_relay_header;

/// Writes h into out[0..12).
void shim_relay_header_put(uint8_t *out, const shim_relay_header *h);

/// Parses a header; returns 0, or -1 for a short datagram, bad magic or another version.
int shim_relay_header_get(const uint8_t *b, size_t len, shim_relay_header *h);

/// LobbyID: the first 4 bytes, big-endian, of SHA-256("udsrelay lobby id\0" + key).
uint32_t shim_relay_lobby_id(const char *key, size_t key_len);

/// Appends the tag to the len-byte datagram in buf. Returns the sealed length, or 0 if it does
/// not fit in cap.
size_t shim_relay_seal(const void *key, size_t key_len, uint8_t *buf, size_t len, size_t cap);

/// Checks a sealed datagram's tag in constant time. Returns the length without the tag, or -1.
long shim_relay_open(const void *key, size_t key_len, const uint8_t *buf, size_t len);

/// Builds a sealed HELLO for the lobby into out. Returns its length, or 0 if cap is too small.
size_t shim_relay_hello(const char *key, size_t key_len, uint32_t session, uint16_t seq, uint8_t *out,
                        size_t cap);
