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

# Regenerates the simulated console log that the Go udstrace tests parse.
fixtures: build/test_core
	./build/test_core $(FIXTURE)

clean:
	rm -rf build
