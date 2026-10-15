#include "shim/ring.h"

#include <string.h>

void shim_ring_init(shim_ring *r, void *mem, size_t cap) {
    r->buf = (uint8_t *)mem;
    r->cap = cap;
    r->head = r->tail = r->used = 0;
    r->dropped_lines = 0;
}

int shim_ring_push(shim_ring *r, const char *line, size_t len) {
    if (len > r->cap - r->used) {
        r->dropped_lines++;
        return -1;
    }
    const size_t first = len < r->cap - r->head ? len : r->cap - r->head;
    memcpy(r->buf + r->head, line, first);
    memcpy(r->buf, line + first, len - first);
    r->head = (r->head + len) % r->cap;
    r->used += len;
    return 0;
}

size_t shim_ring_pop(shim_ring *r, char *out, size_t cap) {
    const size_t n = cap < r->used ? cap : r->used;
    const size_t first = n < r->cap - r->tail ? n : r->cap - r->tail;
    memcpy(out, r->buf + r->tail, first);
    memcpy(out + first, r->buf, n - first);
    r->tail = (r->tail + n) % r->cap;
    r->used -= n;
    return n;
}
