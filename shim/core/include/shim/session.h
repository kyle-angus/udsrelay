// Passthrough MITM for one intercepted service session.
//
// The game holds the client end of a session pair whose server end the shim serves. Each
// request is forwarded unchanged to the real service session: by the time svcReplyAndReceive
// returns, the kernel has already translated the request's handles and buffers into this
// process, so the received command buffer is valid to send on as-is, and the real service's
// reply is valid to send back to the game as-is. The shim's only jobs are to keep the static
// receive buffers right for each direction, close the handle duplicates it receives, and log.
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "shim/ipc.h"
#include "shim/trace.h"

#ifdef __cplusplus
extern "C" {
#endif

/// svcReplyAndReceive result when the session's client end was closed.
#define SHIM_ERR_SESSION_CLOSED ((shim_result_t)0xC920181A)

/// Kernel and platform operations. On the console these wrap svcs and TLS; tests fake them.
typedef struct {
    void *ctx;
    /// The calling thread's IPC command buffer (TLS + 0x80).
    uint32_t *(*cmdbuf)(void *ctx);
    /// The calling thread's receive static buffer descriptors (TLS + 0x180, 32 words).
    uint32_t *(*static_bufs)(void *ctx);
    shim_result_t (*send_sync)(void *ctx, shim_handle_t session);
    shim_result_t (*reply_and_receive)(void *ctx, int32_t *index, const shim_handle_t *handles,
                                       int32_t count, shim_handle_t reply_target);
    void (*close_handle)(void *ctx, shim_handle_t handle);
    /// Address as the kernel sees it, for a pointer into this process (identity on console).
    uint32_t (*addr_of)(void *ctx, const void *ptr);
    /// Readable view of `size` bytes at kernel address `addr`, or NULL (identity on console).
    const uint8_t *(*view)(void *ctx, uint32_t addr, uint32_t size);
    shim_time (*now)(void *ctx);
    void (*log)(void *ctx, const char *line, size_t len);
    /// Optional. Called with each request before it is forwarded; returning nonzero means it
    /// wrote the reply into cmd itself and the real service never sees the request.
    int (*answer)(void *ctx, const char *port, uint32_t *cmd);
} shim_ops;

typedef struct {
    const char *port;     ///< service name, e.g. "nwm::UDS"
    int sess;             ///< logged as sess= to tell concurrent sessions apart; < 0 omits it
    shim_handle_t server; ///< server end of the pair given to the game
    shim_handle_t real;   ///< session to the real service
    const char *proc;     ///< logged as proc=, e.g. the title ID
    long payload_max;     ///< bytes of send/recv/beacon payloads to log, -1 for all
    /// Two banks of SHIM_STATIC_SLOTS receive buffers, slot_size bytes each: one receives the
    /// game's request static buffers, the other the real service's reply static buffers.
    uint8_t *bank_req;
    uint8_t *bank_rsp;
    size_t slot_size;
    char *line; ///< scratch, SHIM_TRACE_LINE_MAX bytes

    uint32_t requests;
    uint32_t failures; ///< forwarded requests whose send failed
    uint32_t answered; ///< requests ops->answer replied to without forwarding
} shim_session;

/// Serves the session until the game closes it (returns 0) or the kernel reports an error
/// that does not clear (returns that error). Closes `server` and `real` before returning.
shim_result_t shim_session_run(shim_session *s, const shim_ops *ops);

/// Points the thread's receive static buffers at `bank` (SHIM_STATIC_SLOTS * slot_size bytes).
void shim_session_set_static_bufs(const shim_ops *ops, uint8_t *bank, size_t slot_size);

#ifdef __cplusplus
}
#endif
