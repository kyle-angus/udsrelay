// SHA-1 (FIPS 180-4) for the hash in a synthesised UDS beacon's network-info tag.
#pragma once

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SHIM_SHA1_SIZE 20

void shim_sha1(const void *data, size_t n, uint8_t out[SHIM_SHA1_SIZE]);

#ifdef __cplusplus
}
#endif
