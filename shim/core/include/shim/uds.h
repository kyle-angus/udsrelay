// nwm::UDS emulation over the relay (Phase 4 relay mode): the C port of internal/udsfake/console.go.
//
// The shim answers the game's nwm::UDS requests itself instead of forwarding them to NWM. One
// shim_uds serves every nwm::UDS session of the process (X/Y keeps two busy at once): each
// session thread passes its requests to shim_uds_handle, which rewrites the command buffer into
// the reply. A separate network thread feeds datagrams from the relay to shim_uds_net_input and
// calls shim_uds_net_poll every 50 ms or so (retries, keepalives, HELLO).
//
// Every entry point takes ops->lock itself. Requests that need a relay round trip (host, scan,
// join, leave, app data, eject) block the session thread in ops->wait, which must release the
// lock while it sleeps so the network thread and the other session keep running.
//
// Scans come back as 802.11 beacons in Azahar's layout (uds_beacon.cpp), which X/Y accepts. The
// node list that NWM would encrypt goes into the beacon's "encrypted" tags in plain text, and
// DecryptBeaconData, which the shim also answers, reads it back from there.
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "shim/ipc.h"
#include "shim/relay_client.h"

#ifdef __cplusplus
extern "C" {
#endif

/// Hardware's result for DestroyNetwork with no network, and PullPacket once the network is gone.
#define SHIM_UDS_RESULT_INVALID_STATE 0xC8A113EAu
/// PullPacket on a bind that does not exist while in a network (Download Play's receive thread,
/// woken by Unbind before DestroyNetwork, in mk7-trace-hw-01).
#define SHIM_UDS_RESULT_NO_BIND 0xE10113EAu
/// A refused or failed host or join. No trace shows NWM's code; this reuses invalid state.
#define SHIM_UDS_RESULT_CONNECT_FAILED SHIM_UDS_RESULT_INVALID_STATE
/// InitializeWithVersion when the relay does not answer: what real NWM returned on hardware when
/// the console was not in local mode, which X/Y reports as "communication failed".
#define SHIM_UDS_RESULT_NO_RELAY 0xE06113F9u
/// Azahar's codes for an unknown node and for running out of binds (module UDS).
#define SHIM_UDS_RESULT_NOT_FOUND 0xC90113FAu
#define SHIM_UDS_RESULT_OUT_OF_MEMORY 0xC86113F3u

/// Wi-Fi channel GetChannel reports while in a network (11 on hardware).
#define SHIM_UDS_CHANNEL 11
/// Bytes of scratch each session passes to shim_uds_handle for static-buffer replies: the
/// largest is PullPacket's frame, which hardware always describes as 0x5C8 bytes.
#define SHIM_UDS_OUT_SIZE 0x5C8
#define SHIM_UDS_MAX_FRAME 0x5C8
#define SHIM_UDS_MAX_BINDS 16
/// Received frames wait in bind queues in fixed chunks from one pool: 512 chunks of 120 bytes
/// hold about 64 KB, several times one X/Y bind's 0x2E30-byte buffer. Each bind is limited to
/// the buffer size the game asked for, as NWM limits it, not to a frame count.
#define SHIM_UDS_CHUNK_DATA 120
#define SHIM_UDS_CHUNKS 512
#define SHIM_UDS_MAX_SCAN 8
/// One network as the relay sends it: struct, u16 bitmask, a NodeInfo per node.
#define SHIM_UDS_NETWORK_MAX (SHIM_RELAY_NETINFO_SIZE + 2 + SHIM_RELAY_MAX_NODES * SHIM_RELAY_NODEINFO_SIZE)

/// GetConnectionStatus word 0.
enum {
    SHIM_UDS_NOT_CONNECTED = 3,
    SHIM_UDS_HOST = 6,
    SHIM_UDS_CONNECTING = 7,
    SHIM_UDS_CLIENT = 9,
    SHIM_UDS_SPECTATOR = 10,
};

/// Status-change reasons. Hardware reports 2 after DisconnectNetwork or DestroyNetwork.
enum {
    SHIM_UDS_REASON_NONE = 0,
    SHIM_UDS_REASON_ESTABLISHED = 1,
    SHIM_UDS_REASON_LEFT = 2,
    SHIM_UDS_REASON_LOST = 4,
};

typedef struct {
    void *ctx;
    uint64_t (*now_ms)(void *ctx); ///< any monotonic clock
    void (*lock)(void *ctx);
    void (*unlock)(void *ctx);
    /// Called with the lock held: releases it, sleeps until wake() or `ms` pass, retakes it.
    void (*wait)(void *ctx, uint32_t ms);
    /// Wakes every thread in wait().
    void (*wake)(void *ctx);
    /// Sends one datagram to the relay.
    void (*send)(void *ctx, const uint8_t *b, size_t n);
    /// Creates an event (one-shot, as NWM's are) and returns its handle, or 0.
    shim_handle_t (*create_event)(void *ctx);
    void (*signal_event)(void *ctx, shim_handle_t h);
    void (*close_handle)(void *ctx, shim_handle_t h);
    /// Address as the kernel sees it, for a pointer into this process (identity on console).
    uint32_t (*addr_of)(void *ctx, const void *p);
    /// Writable view of `size` bytes at a buffer address from a request, or NULL.
    uint8_t *(*mem)(void *ctx, uint32_t addr, uint32_t size);
    void (*log)(void *ctx, const char *level, const char *msg);
} shim_uds_ops;

typedef struct {
    uint32_t status, reason;
    uint16_t node_id, changed;
    uint8_t total, max;
    uint16_t bitmask;
} shim_uds_status;

/// One chunk of a queued frame. A frame's first chunk also carries its source, its length and
/// the link to the next frame in the queue.
typedef struct {
    uint16_t next;       ///< next chunk of the same frame, or 0xFFFF
    uint16_t next_frame; ///< first chunk of the next queued frame, or 0xFFFF (first chunks only)
    uint16_t src, len;   ///< first chunks only
    uint8_t data[SHIM_UDS_CHUNK_DATA];
} shim_uds_chunk;

typedef struct {
    int used;
    uint32_t id;
    uint8_t channel;
    uint16_t node; ///< 0xFFFF: from any node
    uint32_t buf_size, queued_bytes, count, received;
    uint16_t head, tail; ///< first chunks of the oldest and newest queued frames
    shim_handle_t event;
} shim_uds_bind;

typedef struct {
    const shim_uds_ops *ops;
    shim_relay_client relay;
    shim_relay_ops relay_ops;
    int busy;            ///< a session thread owns the relay's request slot
    uint8_t resp[SHIM_RELAY_MAX_DATAGRAM]; ///< the last relay_call's response payload
    size_t resp_len;
    int hello_started;
    uint64_t last_hello;

    int inited;
    uint8_t self[SHIM_RELAY_NODEINFO_SIZE];
    uint8_t mac[6];
    shim_uds_status st;
    uint8_t netinfo[SHIM_RELAY_NETINFO_SIZE];
    uint8_t nodes[SHIM_RELAY_MAX_NODES][SHIM_RELAY_NODEINFO_SIZE];
    shim_handle_t status_event;
    uint32_t word23;

    shim_uds_bind binds[SHIM_UDS_MAX_BINDS];
    shim_uds_chunk chunks[SHIM_UDS_CHUNKS];
    uint16_t free_head, free_count;

    // The scan in flight (filled by the relay's on_scan callback).
    uint8_t scan[SHIM_UDS_MAX_SCAN][SHIM_UDS_NETWORK_MAX];
    uint16_t scan_len[SHIM_UDS_MAX_SCAN];
    int nscan;

    struct {
        uint32_t status_signals, bind_signals, dropped, unknown, relay_failures;
    } stats;
} shim_uds;

/// Sets up the emulator and its relay client. key is the lobby key; session is a random non-zero
/// relay session ID. Returns 0, or -1 for a bad key or session. HELLO starts on the first poll.
int shim_uds_init(shim_uds *u, const shim_uds_ops *ops, const char *key, size_t key_len, uint32_t session);

/// Uses mac for hosted networks and beacons instead of the made-up one shim_uds_init picks.
/// Download Play needs the console's real MAC: it seeds its frame checksums with it.
void shim_uds_set_mac(shim_uds *u, const uint8_t mac[6]);

/// Answers one nwm::UDS request: `cmd` is the received command buffer (64 words) and is
/// rewritten into the reply. `out` is SHIM_UDS_OUT_SIZE bytes of the calling session's own, which
/// static-buffer replies point at; it must stay untouched until the reply has been sent. Request
/// copy handles are left for the caller to close; reply handles stay owned by the emulator.
void shim_uds_handle(shim_uds *u, uint32_t *cmd, uint8_t *out);

/// One datagram from the relay.
void shim_uds_net_input(shim_uds *u, const uint8_t *b, size_t n);

/// Retries, keepalives and HELLO. Call every 50 ms or so.
void shim_uds_net_poll(shim_uds *u);

/// Status without clearing the changed mask (tests and diagnostics).
void shim_uds_peek(shim_uds *u, shim_uds_status *st);

/// Frames queued on a bind; -1 when not in a network, -2 when there is no such bind (PullPacket
/// would then fail with SHIM_UDS_RESULT_INVALID_STATE or SHIM_UDS_RESULT_NO_BIND).
int shim_uds_queued(shim_uds *u, uint32_t bind_id);

/// Nonzero once the relay has answered HELLO.
int shim_uds_relay_up(shim_uds *u);

// Beacon synthesis, exposed for tests.

/// A network as the relay sends it, split up. nodes[i] is node i+1's NodeInfo, or NULL.
typedef struct {
    const uint8_t *netinfo;
    uint16_t bitmask;
    const uint8_t *nodes[SHIM_RELAY_MAX_NODES];
} shim_uds_network;

/// Splits a relay network. Returns 0, or -1 if it is truncated.
int shim_uds_parse_network(const uint8_t *b, size_t n, shim_uds_network *out);

/// Writes one beacon entry (BeaconEntryHeader + frame) for a network. Returns its length, or 0
/// if it does not fit in cap.
size_t shim_uds_beacon_entry(uint8_t *out, size_t cap, const shim_uds_network *net);

/// DecryptBeaconData: the node list in a beacon's two data tags (each starting at the OUI) for a
/// network struct, as NodeInfo[16] (0x280 bytes) with NetworkNodeID filled in.
void shim_uds_decrypt_nodes(const uint8_t netinfo[SHIM_RELAY_NETINFO_SIZE], const uint8_t *tag0, size_t n0,
                            const uint8_t *tag1, size_t n1, uint8_t out[SHIM_RELAY_MAX_NODES * SHIM_RELAY_NODEINFO_SIZE]);

#ifdef __cplusplus
}
#endif
