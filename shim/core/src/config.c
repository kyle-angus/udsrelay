#include "shim/config.h"

#include <ctype.h>
#include <stdlib.h>
#include <string.h>

static void set_ports(shim_config *cfg, const char *list, size_t len) {
    cfg->nports = 0;
    size_t i = 0;
    while (i < len && cfg->nports < SHIM_MAX_PORTS) {
        while (i < len && (list[i] == ',' || isspace((unsigned char)list[i]))) {
            i++;
        }
        size_t start = i;
        while (i < len && list[i] != ',') {
            i++;
        }
        size_t end = i;
        while (end > start && isspace((unsigned char)list[end - 1])) {
            end--;
        }
        if (end > start && end - start <= SHIM_PORT_NAME_MAX) {
            memcpy(cfg->ports[cfg->nports], list + start, end - start);
            cfg->ports[cfg->nports][end - start] = '\0';
            cfg->nports++;
        }
    }
}

void shim_config_defaults(shim_config *cfg) {
    static const char defaults[] = "nwm::UDS,ndm:u";
    memset(cfg, 0, sizeof(*cfg));
    set_ports(cfg, defaults, sizeof(defaults) - 1);
    cfg->payload_max = 512;
}

int shim_config_parse(shim_config *cfg, const char *text, size_t len) {
    int bad = 0;
    size_t i = 0;
    while (i < len) {
        size_t line_end = i;
        while (line_end < len && text[line_end] != '\n') {
            line_end++;
        }
        size_t end = line_end;
        for (size_t j = i; j < line_end; j++) {
            if (text[j] == '#') {
                end = j;
                break;
            }
        }
        while (i < end && isspace((unsigned char)text[i])) {
            i++;
        }
        while (end > i && isspace((unsigned char)text[end - 1])) {
            end--;
        }
        if (end > i) {
            const char *eq = memchr(text + i, '=', end - i);
            if (!eq) {
                bad++;
            } else {
                size_t klen = (size_t)(eq - (text + i));
                while (klen > 0 && isspace((unsigned char)text[i + klen - 1])) {
                    klen--;
                }
                const char *v = eq + 1;
                while (v < text + end && isspace((unsigned char)*v)) {
                    v++;
                }
                const size_t vlen = (size_t)(text + end - v);
                if (klen == 5 && memcmp(text + i, "ports", 5) == 0) {
                    set_ports(cfg, v, vlen);
                } else if (klen == 11 && memcmp(text + i, "payload_max", 11) == 0) {
                    char num[24];
                    const size_t n = vlen < sizeof(num) - 1 ? vlen : sizeof(num) - 1;
                    memcpy(num, v, n);
                    num[n] = '\0';
                    char *endp;
                    const long value = strtol(num, &endp, 10);
                    if (endp == num || *endp != '\0') {
                        bad++;
                    } else {
                        cfg->payload_max = value < 0 ? -1 : value;
                    }
                } else {
                    bad++;
                }
            }
        }
        i = line_end + 1;
    }
    return bad;
}

int shim_config_traces(const shim_config *cfg, const char *port) {
    for (int i = 0; i < cfg->nports; i++) {
        if (strncmp(cfg->ports[i], port, SHIM_PORT_NAME_MAX + 1) == 0) {
            return 1;
        }
    }
    return 0;
}
