#!/bin/sh

set -e
gcc $(go env GOGCCFLAGS) -shared -std=gnu99 lua/lua_sandbox.c -llua -o libsandbox.so
go build lua/lua_sandbox.go
export LD_LIBRARY_PATH=$(pwd)
go test lua/lua_sandbox_test.go
#rm -f libsandbox.so
