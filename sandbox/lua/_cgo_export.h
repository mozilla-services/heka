/* Created by cgo - DO NOT EDIT. */

#line 16 "lua_sandbox.go"
#include <stdlib.h>
#include "lua_sandbox.h"



typedef signed char GoInt8;
typedef unsigned char GoUint8;
typedef short GoInt16;
typedef unsigned short GoUint16;
typedef int GoInt32;
typedef unsigned int GoUint32;
typedef long long GoInt64;
typedef unsigned long long GoUint64;
typedef GoInt64 GoInt;
typedef GoUint64 GoUint;
typedef __SIZE_TYPE__ GoUintptr;
typedef float GoFloat32;
typedef double GoFloat64;
typedef __complex float GoComplex64;
typedef __complex double GoComplex128;

typedef struct { char *p; int n; } GoString;
typedef void *GoMap;
typedef void *GoChan;
typedef struct { void *t; void *v; } GoInterface;
typedef struct { void *data; int len; int cap; } GoSlice;


/* Return type for go_lua_read_message */
struct go_lua_read_message_return {
	GoInt r0;
	void* r1;
	GoInt r2;
};

extern struct go_lua_read_message_return go_lua_read_message(void* p0, char* p1, GoInt p2, GoInt p3);

extern void go_lua_inject_message(void* p0, char* p1);
