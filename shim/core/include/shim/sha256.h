// SHA-256 and HMAC-SHA256 (FIPS 180-4, RFC 2104) for the relay's datagram tags. Small and
// dependency-free so the console build needs nothing beyond libc.
#pragma once

#include <stddef.h>
#include <stdint.h>

#define SHIM_SHA256_SIZE 32
#define SHIM_SHA256_BLOCK 64

typedef struct {
    uint32_t h[8];
    uint64_t len; ///< bytes hashed so far
    uint8_t buf[SHIM_SHA256_BLOCK];
    size_t used;
} shim_sha256_ctx;

void shim_sha256_init(shim_sha256_ctx *s);
void shim_sha256_update(shim_sha256_ctx *s, const void *data, size_t n);
void shim_sha256_final(shim_sha256_ctx *s, uint8_t out[SHIM_SHA256_SIZE]);
void shim_sha256(const void *data, size_t n, uint8_t out[SHIM_SHA256_SIZE]);

void shim_hmac_sha256(const void *key, size_t key_len, const void *data, size_t n,
                      uint8_t out[SHIM_SHA256_SIZE]);
