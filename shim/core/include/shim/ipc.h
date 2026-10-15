// 3DS IPC message parsing. Host-agnostic: builds for the console and for PC tests.
#pragma once

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef int32_t shim_result_t;
typedef uint32_t shim_handle_t;

/// IPC command buffers are 0x100 bytes (64 words) at TLS+0x80.
#define SHIM_IPC_MAX_WORDS 64
/// Receive static buffer descriptors live at TLS+0x180: 16 (descriptor, pointer) pairs.
#define SHIM_STATIC_SLOTS 16

#define SHIM_IPC_CMD(hdr) ((uint16_t)((hdr) >> 16))
#define SHIM_IPC_NORMAL(hdr) (((hdr) >> 6) & 0x3F)
#define SHIM_IPC_TRANSLATE(hdr) ((hdr) & 0x3F)
#define SHIM_IPC_MAKE_HEADER(cmd, normal, translate) \
    (((uint32_t)(cmd) << 16) | (((uint32_t)(normal) & 0x3F) << 6) | ((uint32_t)(translate) & 0x3F))
#define SHIM_IPC_STATIC_DESC(size, id) (((uint32_t)(size) << 14) | (((uint32_t)(id) & 0xF) << 10) | 0x2)

typedef enum {
    SHIM_DESC_COPY_HANDLES,  ///< handles duplicated into the receiver
    SHIM_DESC_MOVE_HANDLES,  ///< handles moved to the receiver
    SHIM_DESC_PROCESS_ID,    ///< kernel writes the sender's process ID
    SHIM_DESC_STATIC_BUFFER, ///< data copied into the receiver's static buffer `id`
    SHIM_DESC_PXI_BUFFER,
    SHIM_DESC_MAPPED_BUFFER, ///< buffer mapped into the receiver
} shim_desc_kind;

typedef struct {
    shim_desc_kind kind;
    /// Word index of the descriptor itself; payload words follow it.
    uint8_t word;
    /// Handle count for handle descriptors.
    uint8_t count;
    /// Static buffer ID, or mapped buffer permission bits (1 = read, 2 = write).
    uint8_t id;
    uint32_t size;
    uint32_t ptr;
} shim_desc;

#define SHIM_MAX_DESCS 16

typedef struct {
    uint32_t header;
    uint16_t cmd;
    uint8_t normal;
    uint8_t translate;
    uint8_t ndesc;
    shim_desc desc[SHIM_MAX_DESCS];
} shim_ipc_msg;

/// Total words in the message including the header.
static inline unsigned shim_ipc_words(uint32_t header) {
    return 1 + SHIM_IPC_NORMAL(header) + SHIM_IPC_TRANSLATE(header);
}

/// Parses `words` (at least shim_ipc_words(words[0]) long). Returns 0, or -1 if the
/// translate section is malformed or overflows the command buffer.
int shim_ipc_parse(const uint32_t *words, shim_ipc_msg *out);

#ifdef __cplusplus
}
#endif
