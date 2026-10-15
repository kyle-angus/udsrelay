#include "shim/ipc.h"

int shim_ipc_parse(const uint32_t *words, shim_ipc_msg *out) {
    const uint32_t header = words[0];
    out->header = header;
    out->cmd = SHIM_IPC_CMD(header);
    out->normal = (uint8_t)SHIM_IPC_NORMAL(header);
    out->translate = (uint8_t)SHIM_IPC_TRANSLATE(header);
    out->ndesc = 0;

    const unsigned total = shim_ipc_words(header);
    if (total > SHIM_IPC_MAX_WORDS) {
        return -1;
    }

    unsigned i = 1 + out->normal;
    while (i < total) {
        if (out->ndesc == SHIM_MAX_DESCS) {
            return -1;
        }
        const uint32_t d = words[i];
        shim_desc *desc = &out->desc[out->ndesc];
        desc->word = (uint8_t)i;
        desc->count = 0;
        desc->id = 0;
        desc->size = 0;
        desc->ptr = 0;

        unsigned payload;
        if ((d & 0xF) == 0) {
            // Handle descriptor: bit 4 = move, bit 5 = calling process ID, count in bits 26-31.
            if (d & 0x20) {
                desc->kind = SHIM_DESC_PROCESS_ID;
                payload = 1;
            } else {
                desc->kind = (d & 0x10) ? SHIM_DESC_MOVE_HANDLES : SHIM_DESC_COPY_HANDLES;
                payload = (d >> 26) + 1;
            }
            desc->count = (uint8_t)payload;
        } else if ((d & 0xF) == 0x2) {
            desc->kind = SHIM_DESC_STATIC_BUFFER;
            desc->size = d >> 14;
            desc->id = (uint8_t)((d >> 10) & 0xF);
            payload = 1;
        } else if ((d & 0xF) == 0x4 || (d & 0xF) == 0x6) {
            desc->kind = SHIM_DESC_PXI_BUFFER;
            desc->size = d >> 8;
            desc->id = (uint8_t)((d >> 4) & 0xF);
            payload = 1;
        } else if (d & 0x8) {
            desc->kind = SHIM_DESC_MAPPED_BUFFER;
            desc->size = d >> 4;
            desc->id = (uint8_t)((d >> 1) & 0x3);
            payload = 1;
        } else {
            return -1;
        }

        if (i + 1 + payload > total) {
            return -1;
        }
        if (desc->kind != SHIM_DESC_COPY_HANDLES && desc->kind != SHIM_DESC_MOVE_HANDLES &&
            desc->kind != SHIM_DESC_PROCESS_ID) {
            desc->ptr = words[i + 1];
        }
        out->ndesc++;
        i += 1 + payload;
    }
    return 0;
}
