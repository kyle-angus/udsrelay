// Locates the game's srv:GetServiceHandle client function in its ARM code.
#pragma once

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/// srv:GetServiceHandle request header: command 5, 4 normal words, no translate words.
#define SHIM_SRV_GET_SERVICE_HANDLE_HDR 0x00050100u

typedef struct {
    uint32_t func;    ///< address of the function's first instruction (the push)
    uint32_t literal; ///< address of the 0x00050100 literal it loads
    uint32_t svc;     ///< address of its svc 0x32 (SendSyncRequest)
} shim_hook_candidate;

/// Scans ARM code `text` (`words` long, loaded at `base`) for functions that load the
/// GetServiceHandle header from a literal pool, start with a push that saves lr, and issue
/// svc 0x32 before returning. The 3DS has no movw, so the 0x00050100 constant must come from
/// a literal pool. Returns the number of candidates written to `out` (at most `max`).
size_t shim_find_get_service_handle(const uint32_t *text, size_t words, uint32_t base,
                                    shim_hook_candidate *out, size_t max);

/// Picks which hook argument is the `Handle *out` the original function filled in: the one
/// (among the 4 register arguments) that points at a readable word equal to `handle`.
/// `read_word` returns 0 and sets *value when `addr` is a valid, writable, aligned word.
/// Returns the argument index 0-3, or -1 if none or more than one distinct pointer matches.
int shim_pick_out_arg(const uint32_t args[4], uint32_t handle,
                      int (*read_word)(void *ctx, uint32_t addr, uint32_t *value), void *ctx);

#ifdef __cplusplus
}
#endif
