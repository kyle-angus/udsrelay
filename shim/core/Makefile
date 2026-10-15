# Host build of the shim core for tests. The console build compiles src/*.c into the plugin.
CC ?= cc
CFLAGS ?= -std=c11 -O1 -g -Wall -Wextra -Werror -fsanitize=address,undefined
SRC := $(wildcard src/*.c)
FIXTURE := ../../internal/udstrace/testdata/console_host.log

.PHONY: test fixtures clean

build/test_core: $(SRC) tests/test_core.c $(wildcard include/shim/*.h)
	@mkdir -p build
	$(CC) $(CFLAGS) -Iinclude $(SRC) tests/test_core.c -o $@

test: build/test_core
	./build/test_core

# PC-only probe: the relay client over a real UDP socket. internal/relay/probe_test.go builds and
# runs it against the Go relay.
build/relay_probe: $(SRC) tests/relay_probe.c $(wildcard include/shim/*.h)
	@mkdir -p build
	$(CC) $(CFLAGS) -Iinclude $(SRC) tests/relay_probe.c -o $@

# PC-only: the UDS emulator as one console, for trace replay. internal/udsfake (ShimConsole) builds
# and drives it against the Go relay.
build/uds_replay: $(SRC) tests/uds_replay.c $(wildcard include/shim/*.h)
	@mkdir -p build
	$(CC) $(CFLAGS) -Iinclude $(SRC) tests/uds_replay.c -lpthread -o $@

# Regenerates the simulated console log that the Go udstrace tests parse.
fixtures: build/test_core
	./build/test_core $(FIXTURE)

clean:
	rm -rf build
