#!/bin/sh

set -e
gcc $(go env GOGCCFLAGS) -shared -std=gnu99 lua_sandbox.c -llua -o liblua_sandbox.so
go build lua_sandbox.go
LD_LIBRARY_PATH=./
go test lua_sandbox_test.go
rm -f liblua_sandbox.so
