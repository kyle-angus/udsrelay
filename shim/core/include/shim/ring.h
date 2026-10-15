// Byte ring for log lines: producers push whole lines, the SD writer drains bytes.
// Not thread-safe; callers hold a lock around every call.
#pragma once

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    uint8_t *buf;
    size_t cap;
    size_t head; ///< next write position
    size_t tail; ///< next read position
    size_t used;
    uint32_t dropped_lines; ///< lines rejected because the ring was full, since the last report
} shim_ring;

void shim_ring_init(shim_ring *r, void *mem, size_t cap);

/// Pushes a whole line or nothing. Returns 0, or -1 (and counts a dropped line) if full.
int shim_ring_push(shim_ring *r, const char *line, size_t len);

/// Copies up to `cap` bytes out in order. Returns the number of bytes copied.
size_t shim_ring_pop(shim_ring *r, char *out, size_t cap);

#ifdef __cplusplus
}
#endif
