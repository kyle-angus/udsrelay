// Console side of the relay: the C counterpart of internal/relay/client.go.
//
// A single-threaded state machine with no sockets or threads of its own. The caller feeds it
// every datagram from the relay (shim_relay_input), calls shim_relay_poll regularly (every 50 ms
// or so) for retries, timeouts and keepalives, and serialises all calls with its own lock.
// Datagrams go out through ops->send; times are milliseconds from any monotonic clock.
//
// One request (HELLO, HOST_CREATE, HOST_UPDATE, SCAN_REQ, JOIN_REQ, EJECT, LEAVE) is in flight
// at a time. It is retried every retry_ms until its response arrives or timeout_ms passes; then
// shim_relay_request_state reports DONE (with the response's status and payload) or FAILED.
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "shim/relay_wire.h"

#ifdef __cplusplus
extern "C" {
#endif

#define SHIM_RELAY_NETINFO_SIZE 0x108
#define SHIM_RELAY_NODEINFO_SIZE 0x28
#define SHIM_RELAY_MAX_NODES 16

/// Response status codes (the first payload byte of every response).
enum {
    SHIM_RELAY_STATUS_OK = 0,
    SHIM_RELAY_STATUS_NOT_FOUND = 1,
    SHIM_RELAY_STATUS_FULL = 2,
    SHIM_RELAY_STATUS_PASSPHRASE = 3,
    SHIM_RELAY_STATUS_BAD_REQUEST = 4,
    SHIM_RELAY_STATUS_NOT_HOST = 5,
};

/// NODE_EVENT kinds and leave reasons.
enum { SHIM_RELAY_EV_JOINED = 1, SHIM_RELAY_EV_LEFT = 2, SHIM_RELAY_EV_EJECTED = 3, SHIM_RELAY_EV_CLOSED = 4 };
enum { SHIM_RELAY_REASON_CLEAN = 0, SHIM_RELAY_REASON_TIMEOUT = 1, SHIM_RELAY_REASON_EJECTED = 2 };

/// HOST_UPDATE kinds, and JOIN connection types.
enum { SHIM_RELAY_UPDATE_APPDATA = 1, SHIM_RELAY_UPDATE_ATTRIBUTES = 2 };
enum { SHIM_RELAY_CONN_CLIENT = 1, SHIM_RELAY_CONN_SPECTATOR = 2 };

typedef enum {
    SHIM_RELAY_REQ_IDLE,
    SHIM_RELAY_REQ_PENDING,
    SHIM_RELAY_REQ_DONE,   ///< a response arrived: see status and the response payload
    SHIM_RELAY_REQ_FAILED, ///< no response before the timeout, or the relay reset our session
} shim_relay_req_state;

typedef struct {
    uint8_t kind, reason;
    uint32_t network_id;
    uint16_t node, bitmask;
    uint8_t total;
    uint8_t info[SHIM_RELAY_NODEINFO_SIZE]; ///< joining node's NodeInfo; zero for other kinds
} shim_relay_event;

typedef struct {
    void *ctx;
    /// Sends one datagram to the relay. Errors are ignored: requests retry, data is unreliable.
    void (*send)(void *ctx, const uint8_t *b, size_t n);
    void (*on_data)(void *ctx, uint16_t src, uint16_t dst, uint8_t channel, uint8_t flags, const uint8_t *payload,
                    size_t n);
    void (*on_event)(void *ctx, const shim_relay_event *e);
    /// The relay lost our session (restart or timeout): any network is gone. The client says
    /// HELLO again on its own.
    void (*on_reset)(void *ctx);
    /// One network from a scan in flight, once per index: the 0x108-byte struct, u16 bitmask and
    /// a NodeInfo per set bit, as in relay/RUNBOOK.md.
    void (*on_scan)(void *ctx, uint8_t index, uint8_t total, const uint8_t *network, size_t n);
} shim_relay_ops;

typedef struct {
    const shim_relay_ops *ops;
    char key[SHIM_RELAY_MAX_LOBBY_KEY];
    size_t key_len;
    uint32_t session;
    uint16_t seq; ///< last seq sent

    uint32_t retry_ms, timeout_ms, keepalive_ms;
    int hello_ok;   ///< HELLO answered; keepalives run
    int need_hello; ///< say HELLO when no request is in flight (after a reset)
    uint64_t last_keepalive;
    uint32_t rtt_ms;

    // The request in flight.
    shim_relay_req_state req_state;
    uint8_t req_type;
    uint16_t req_seq;
    uint64_t req_start, req_sent;
    uint8_t req[SHIM_RELAY_MAX_DATAGRAM];
    size_t req_len;
    uint8_t status;                         ///< response status when DONE
    uint8_t resp[SHIM_RELAY_MAX_DATAGRAM];  ///< response payload after the status byte
    size_t resp_len;
    uint32_t scan_got; ///< indexes seen in this scan (first 32)
    uint8_t scan_count, scan_total;

    uint16_t ev_last; ///< newest NODE_EVENT seq delivered
    int ev_any;
    uint16_t in_last; ///< newest seq of the server's other datagrams (data, replies), numbered as one stream
    int in_any;

    struct {
        uint32_t data_in, data_out, bad_tag, retries, events, resets;
        uint32_t lost; ///< datagrams from the server that never arrived (gaps in in_last's stream)
    } stats;
} shim_relay_client;

/// Sets up a client. key is the lobby key (1 to 64 bytes); session must be non-zero and random.
/// Returns 0, or -1 for a bad key or session. Call shim_relay_hello next.
int shim_relay_client_init(shim_relay_client *c, const shim_relay_ops *ops, const char *key, size_t key_len,
                           uint32_t session);

/// Starts a request. payload is everything after the header (built by the helpers below).
/// Returns 0, or -1 if another request is in flight or the payload is too large.
int shim_relay_request(shim_relay_client *c, uint8_t type, const uint8_t *payload, size_t n, uint64_t now);

/// Starts HELLO for the client's lobby.
int shim_relay_hello_request(shim_relay_client *c, uint64_t now);

/// Ends a finished request (DONE or FAILED) so the next can start.
void shim_relay_request_clear(shim_relay_client *c);

/// Handles one datagram from the relay. Datagrams whose tag does not verify are dropped.
void shim_relay_input(shim_relay_client *c, const uint8_t *b, size_t n, uint64_t now);

/// Retransmits the request in flight, times it out, says HELLO again after a reset, and sends
/// keepalives.
void shim_relay_poll(shim_relay_client *c, uint64_t now);

/// Sends one DATA frame (unreliable). Returns 0, or -1 if the payload does not fit.
int shim_relay_send_data(shim_relay_client *c, uint16_t dst, uint8_t channel, uint8_t flags, const uint8_t *payload,
                         size_t n);

// Request payload builders. Each returns the payload length, or 0 if it does not fit in cap.
size_t shim_relay_host_create(uint8_t *out, size_t cap, const uint8_t pass_hash[32],
                              const uint8_t self[SHIM_RELAY_NODEINFO_SIZE], const uint8_t netinfo[SHIM_RELAY_NETINFO_SIZE]);
size_t shim_relay_update_appdata(uint8_t *out, size_t cap, const uint8_t *data, size_t n);
size_t shim_relay_update_attributes(uint8_t *out, size_t cap, uint16_t attributes);
size_t shim_relay_scan(uint8_t *out, size_t cap, uint32_t comm_id, uint8_t id8);
size_t shim_relay_join(uint8_t *out, size_t cap, uint32_t network_id, const uint8_t pass_hash[32], uint8_t conn_type,
                       const uint8_t self[SHIM_RELAY_NODEINFO_SIZE]);
size_t shim_relay_eject(uint8_t *out, size_t cap, uint16_t node);

#ifdef __cplusplus
}
#endif
