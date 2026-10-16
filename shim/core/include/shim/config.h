// Shim settings, read from sdmc:/udsrelay/shim.ini on the console.
#pragma once

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SHIM_MAX_PORTS 4
#define SHIM_PORT_NAME_MAX 8 // service names are at most 8 characters

typedef struct {
    char ports[SHIM_MAX_PORTS][SHIM_PORT_NAME_MAX + 1];
    int nports;
    /// Bytes of each payload to log; -1 logs everything.
    long payload_max;
    /// Relay to check (Phase 4): host name or IPv4 address and UDP port; empty host = none.
    char relay_host[64];
    unsigned short relay_port;
    /// Lobby key shared with the relay and the other console.
    char lobby[64 + 1];
    /// Answer ndm:u's exclusive-state calls locally so infrastructure Wi-Fi stays up.
    int fake_ndm;
    /// mode = relay: answer nwm::UDS in the shim over the relay instead of forwarding it to NWM
    /// (needs relay and lobby; implies fake_ndm). mode = passthrough (the default) forwards.
    int relay_mode;
    /// mode = trace (sysmodule only): forward and log every redirected nwm::UDS and ndm:u
    /// session, the Download Play module's included, to the real services. The plugin treats it
    /// as passthrough.
    int trace_mode;
    /// log_frames = 0, 1 or 2: log none, all, or all but the empty PullPackets of nwm::UDS's
    /// per-frame calls (see shim_session.quiet_frames). -1 when shim.ini does not say; the plugin
    /// then logs them all, the sysmodule in relay mode none.
    int log_frames;
} shim_config;

/// Defaults: ports nwm::UDS and ndm:u, payload_max 512.
void shim_config_defaults(shim_config *cfg);

/// Applies `key = value` lines; '#' starts a comment. Keys: ports (comma-separated),
/// payload_max, relay (host:port), lobby, fake_ndm (0 or 1), mode (passthrough, relay or trace),
/// log_frames (0, 1 or 2).
/// Returns the number of lines it could not understand.
int shim_config_parse(shim_config *cfg, const char *text, size_t len);

/// Nonzero when `port` is one the shim should intercept.
int shim_config_traces(const shim_config *cfg, const char *port);

#ifdef __cplusplus
}
#endif
