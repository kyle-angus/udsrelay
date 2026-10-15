#include "shim/finder.h"

// ldr Rd, [pc, #±imm12]: cond 0101 U001 1111 Rd imm12, any condition.
static int is_ldr_literal(uint32_t ins, uint32_t ins_addr, uint32_t *target) {
    if ((ins & 0x0F7F0000u) != 0x051F0000u || (ins >> 28) == 0xF) {
        return 0;
    }
    const uint32_t imm = ins & 0xFFFu;
    const uint32_t pc = ins_addr + 8;
    *target = (ins & (1u << 23)) ? pc + imm : pc - imm;
    return 1;
}

// stmdb sp!, {..., lr} (push with lr), unconditional.
static int is_push_lr(uint32_t ins) {
    return (ins & 0xFFFF4000u) == 0xE92D4000u;
}

// Unconditional ldmia sp!, {..., pc} (pop with pc) or bx lr: the end of a function.
// Conditional returns are early exits (error paths) and do not end the function.
static int is_return(uint32_t ins) {
    return (ins & 0xFFFF8000u) == 0xE8BD8000u || ins == 0xE12FFF1Eu;
}

#define SVC_SEND_SYNC_REQUEST 0xEF000032u
/// How far back from the literal load to look for the function's push.
#define MAX_PROLOGUE_WORDS 64
/// How far forward from the push to look for svc 0x32 before giving up.
#define MAX_BODY_WORDS 128

size_t shim_find_get_service_handle(const uint32_t *text, size_t words, uint32_t base,
                                    shim_hook_candidate *out, size_t max) {
    size_t found = 0;
    for (size_t i = 0; i < words && found < max; i++) {
        uint32_t target;
        if (!is_ldr_literal(text[i], base + 4 * (uint32_t)i, &target)) {
            continue;
        }
        if (target < base || target >= base + 4 * (uint32_t)words || (target & 3)) {
            continue;
        }
        if (text[(target - base) / 4] != SHIM_SRV_GET_SERVICE_HANDLE_HDR) {
            continue;
        }

        // Walk back to the nearest push {.., lr}; stop at an earlier function's return.
        size_t start = i;
        int have_start = 0;
        for (size_t back = 0; back <= MAX_PROLOGUE_WORDS && back <= i; back++) {
            const uint32_t ins = text[i - back];
            if (back > 0 && is_return(ins)) {
                break;
            }
            if (is_push_lr(ins)) {
                start = i - back;
                have_start = 1;
                break;
            }
        }
        if (!have_start) {
            continue;
        }

        // The body must issue svc 0x32 after loading the header and before returning.
        uint32_t svc = 0;
        for (size_t j = i + 1; j < words && j <= start + MAX_BODY_WORDS; j++) {
            if (text[j] == SVC_SEND_SYNC_REQUEST) {
                svc = base + 4 * (uint32_t)j;
                break;
            }
            if (is_return(text[j])) {
                break;
            }
        }
        if (!svc) {
            continue;
        }

        const uint32_t func = base + 4 * (uint32_t)start;
        int dup = 0;
        for (size_t k = 0; k < found; k++) {
            if (out[k].func == func) {
                dup = 1;
            }
        }
        if (!dup) {
            out[found].func = func;
            out[found].literal = target;
            out[found].svc = svc;
            found++;
        }
    }
    return found;
}

int shim_pick_out_arg(const uint32_t args[4], uint32_t handle,
                      int (*read_word)(void *ctx, uint32_t addr, uint32_t *value), void *ctx) {
    int pick = -1;
    for (int i = 0; i < 4; i++) {
        uint32_t value;
        if (args[i] == 0 || (args[i] & 3) || read_word(ctx, args[i], &value) != 0 ||
            value != handle) {
            continue;
        }
        if (pick >= 0 && args[pick] != args[i]) {
            return -1; // ambiguous
        }
        if (pick < 0) {
            pick = i;
        }
    }
    return pick;
}
