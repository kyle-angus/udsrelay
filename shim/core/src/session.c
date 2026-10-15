#include "shim/session.h"

#include <stdio.h>
#include <string.h>

#define MAX_PENDING_CLOSE 16

void shim_session_set_static_bufs(const shim_ops *ops, uint8_t *bank, size_t slot_size) {
    uint32_t *sb = ops->static_bufs(ops->ctx);
    for (unsigned i = 0; i < SHIM_STATIC_SLOTS; i++) {
        sb[2 * i] = SHIM_IPC_STATIC_DESC(slot_size, i);
        sb[2 * i + 1] = ops->addr_of(ops->ctx, bank + i * slot_size);
    }
}

static void emit(const shim_ops *ops, const char *line, size_t len) {
    if (len) {
        ops->log(ops->ctx, line, len);
    }
}

static void note(const shim_session *s, const shim_ops *ops, const char *level, const char *msg) {
    emit(ops, s->line, shim_trace_note(s->line, SHIM_TRACE_LINE_MAX, ops->now(ops->ctx), level,
                                       "Session", msg));
}

static size_t capped(const shim_session *s, size_t size) {
    if (s->payload_max < 0 || (size_t)s->payload_max >= size) {
        return size;
    }
    return (size_t)s->payload_max;
}

static void blob(const shim_session *s, const shim_ops *ops, uint16_t cmd, const char *tag,
                 const char *extra, uint32_t addr, uint32_t size, int cap) {
    const uint8_t *data = ops->view(ops->ctx, addr, size);
    if (!data) {
        return;
    }
    const size_t logged = cap ? capped(s, size) : size;
    emit(ops, s->line,
         shim_trace_blob(s->line, SHIM_TRACE_LINE_MAX, ops->now(ops->ctx), s->port, s->sess, cmd, tag, extra,
                         data, size, logged));
}

static uint32_t word(const uint32_t *w, unsigned i) {
    return i < shim_ipc_words(w[0]) ? w[i] : 0;
}

// Logs the payloads of a request (reply == 0, req == words) or reply (reply != 0, words is the
// reply and req the original request).
static void log_buffers(const shim_session *s, const shim_ops *ops, const shim_ipc_msg *msg,
                        const uint32_t *words, const uint32_t *req, int reply) {
    const uint16_t cmd = SHIM_IPC_CMD(req[0]);
    char extra[64];
    for (unsigned i = 0; i < msg->ndesc; i++) {
        const shim_desc *d = &msg->desc[i];
        const int mapped = d->kind == SHIM_DESC_MAPPED_BUFFER;
        if (d->kind != SHIM_DESC_STATIC_BUFFER && !mapped) {
            continue;
        }
        const char *tag = shim_trace_tag(s->port, cmd, reply, d->id, mapped);
        char generic[16];
        if (!tag) {
            if (mapped) {
                continue; // mapped buffers are only interesting for beacon replies
            }
            snprintf(generic, sizeof(generic), "%s%u", reply ? "rbuf" : "sbuf", d->id);
            tag = generic;
        }
        uint32_t size = d->size;
        int cap = 0;
        extra[0] = '\0';
        if (strcmp(tag, "send") == 0) {
            snprintf(extra, sizeof(extra), "dest=0x%04x chan=%u flags=0x%02x",
                     (unsigned)(word(req, 2) & 0xFFFF), (unsigned)(word(req, 3) & 0xFF),
                     (unsigned)(word(req, 6) & 0xFF));
            if (word(req, 5) < size) {
                size = word(req, 5); // the descriptor size is the aligned length
            }
            cap = 1;
        } else if (strcmp(tag, "recv") == 0) {
            const uint32_t actual = word(words, 2);
            if (actual == 0) {
                continue; // empty poll
            }
            if (actual < size) {
                size = actual;
            }
            snprintf(extra, sizeof(extra), "bind=%u src=0x%04x", (unsigned)word(req, 1),
                     (unsigned)(word(words, 3) & 0xFFFF));
            cap = 1;
        } else if (strcmp(tag, "netinfo") == 0 && cmd == 0x001E) {
            snprintf(extra, sizeof(extra), "conn_type=%u", (unsigned)(word(req, 1) & 0xFF));
        } else if (strcmp(tag, "beacons") == 0) {
            // nwmBeaconDataReplyHeader: maxsize, size, total_entries.
            const uint8_t *hdr = ops->view(ops->ctx, d->ptr, 12);
            if (!hdr) {
                continue;
            }
            uint32_t used;
            memcpy(&used, hdr + 4, sizeof(used));
            if (used < size) {
                size = used;
            }
            cap = 1;
        }
        blob(s, ops, cmd, tag, extra, d->ptr, size, cap);
    }
}

static int collect_copy_handles(const shim_ipc_msg *msg, const uint32_t *words, shim_handle_t *out,
                                int n) {
    for (unsigned i = 0; i < msg->ndesc; i++) {
        const shim_desc *d = &msg->desc[i];
        if (d->kind != SHIM_DESC_COPY_HANDLES) {
            continue;
        }
        for (unsigned k = 0; k < d->count && n < MAX_PENDING_CLOSE; k++) {
            const shim_handle_t h = words[d->word + 1 + k];
            if (h) {
                out[n++] = h;
            }
        }
    }
    return n;
}

shim_result_t shim_session_run(shim_session *s, const shim_ops *ops) {
    uint32_t req[SHIM_IPC_MAX_WORDS];
    shim_handle_t pending_close[MAX_PENDING_CLOSE];
    int npending = 0;
    shim_handle_t reply_target = 0;
    shim_result_t ret = 0;
    int consecutive_errors = 0;

    for (;;) {
        shim_session_set_static_bufs(ops, s->bank_req, s->slot_size);
        int32_t index = -1;
        shim_result_t rc = ops->reply_and_receive(ops->ctx, &index, &s->server, 1, reply_target);

        // Handles the real service copied to us were passed on by that reply; drop ours.
        for (int i = 0; i < npending; i++) {
            ops->close_handle(ops->ctx, pending_close[i]);
        }
        npending = 0;
        reply_target = 0;

        if (rc == SHIM_ERR_SESSION_CLOSED) {
            break;
        }
        if (rc < 0) {
            char msg[64];
            snprintf(msg, sizeof(msg), "%s: svcReplyAndReceive failed 0x%08x", s->port, (unsigned)rc);
            note(s, ops, "Error", msg);
            if (++consecutive_errors >= 2) {
                ret = rc;
                break;
            }
            continue;
        }
        consecutive_errors = 0;

        uint32_t *cmd = ops->cmdbuf(ops->ctx);
        unsigned nreq = shim_ipc_words(cmd[0]);
        if (nreq > SHIM_IPC_MAX_WORDS) {
            nreq = SHIM_IPC_MAX_WORDS;
        }
        shim_ipc_msg msg;
        if (shim_ipc_parse(cmd, &msg) != 0) {
            // The kernel already accepted this message, so forward it anyway; only our view
            // of its buffers and handles is lost.
            msg.ndesc = 0;
            note(s, ops, "Warning", "could not parse request descriptors; forwarding unlogged");
        }
        memcpy(req, cmd, nreq * sizeof(uint32_t));
        const uint16_t id = msg.cmd;
        s->requests++;

        emit(ops, s->line,
             shim_trace_msg(s->line, SHIM_TRACE_LINE_MAX, ops->now(ops->ctx), "req", s->port, s->sess, id,
                            shim_command_name(s->port, id), 0, s->proc, req, 0));
        log_buffers(s, ops, &msg, req, req, 0);

        // Our duplicates of the game's copy handles; the real service gets its own.
        shim_handle_t req_copies[MAX_PENDING_CLOSE];
        const int nreq_copies = collect_copy_handles(&msg, req, req_copies, 0);

        const int answered = ops->answer && ops->answer(ops->ctx, s, cmd);
        if (answered) {
            rc = 0;
            s->answered++;
            const uint64_t bit = id < 64 ? (uint64_t)1 << id : 0;
            if (!bit || !(s->answered_noted & bit)) {
                s->answered_noted |= bit;
                char m[96];
                snprintf(m, sizeof(m), "%s: answered 0x%04x locally (noted once per command)", s->port, id);
                note(s, ops, "Info", m);
            }
        } else {
            shim_session_set_static_bufs(ops, s->bank_rsp, s->slot_size);
            rc = ops->send_sync(ops->ctx, s->real);
        }

        for (int i = 0; i < nreq_copies; i++) {
            ops->close_handle(ops->ctx, req_copies[i]);
        }

        if (rc < 0) {
            s->failures++;
            cmd[0] = SHIM_IPC_MAKE_HEADER(id, 1, 0);
            cmd[1] = (uint32_t)rc;
            char m[64];
            snprintf(m, sizeof(m), "%s: forwarding 0x%04x failed 0x%08x", s->port, id, (unsigned)rc);
            note(s, ops, "Error", m);
        }

        shim_ipc_msg rsp;
        if (shim_ipc_parse(cmd, &rsp) == 0) {
            emit(ops, s->line,
                 shim_trace_msg(s->line, SHIM_TRACE_LINE_MAX, ops->now(ops->ctx), "rsp", s->port,
                                s->sess, id, NULL, 0, NULL, cmd, 0));
            log_buffers(s, ops, &rsp, cmd, req, 1);
            if (!answered) {
                npending = collect_copy_handles(&rsp, cmd, pending_close, 0);
            }
        }
        reply_target = s->server;
    }

    ops->close_handle(ops->ctx, s->server);
    ops->close_handle(ops->ctx, s->real);
    return ret;
}
