// UDSTRACE v=1 line formatting, matching recon/azahar/udstrace.patch so `udstrace`
// reads console logs and Azahar logs the same way.
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "shim/ipc.h"

#ifdef __cplusplus
extern "C" {
#endif

/// Longest line the formatter produces: header text plus a full payload as hex.
#define SHIM_TRACE_LINE_MAX (512 + 2 * 0x1000)

typedef struct {
    uint64_t wall_us;   ///< microseconds since the Unix epoch
    uint64_t uptime_us; ///< microseconds since the shim started, for the stock-style prefix
} shim_time;

/// Formats a req or rsp line from raw message words. Returns the line length (no newline
/// terminator is added beyond the trailing '\n'), or 0 if `cap` is too small.
size_t shim_trace_msg(char *out, size_t cap, shim_time t, const char *kind, const char *port,
                      uint16_t cmd, const char *name, uint32_t tid, const char *proc,
                      const uint32_t *words, int deferred);

/// Formats a blob line. `size` is the full payload length; only `logged` bytes are hex-encoded.
size_t shim_trace_blob(char *out, size_t cap, shim_time t, const char *port, uint16_t cmd,
                       const char *tag, const char *extra, const uint8_t *data, size_t size,
                       size_t logged);

/// Formats a plugin diagnostic as a stock-style log line of class "Plugin".
size_t shim_trace_note(char *out, size_t cap, shim_time t, const char *level, const char *func,
                       const char *message);

/// Names the payload carried by static/mapped buffer `id` for a command, using the same tags
/// as the Azahar patch where one exists (netinfo, passphrase, appdata, send, recv).
/// `reply` is nonzero for reply buffers. Returns NULL when there is no specific tag.
const char *shim_trace_tag(const char *port, uint16_t cmd, int reply, uint8_t id, int mapped);

/// Azahar's name for a command, or NULL when unknown.
const char *shim_command_name(const char *port, uint16_t cmd);

#ifdef __cplusplus
}
#endif
