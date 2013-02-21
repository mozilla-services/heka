/* Created by cgo - DO NOT EDIT. */

#line 30 "lua_sandbox.go"
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

typedef struct { const char *p; int n; } GoString;
typedef void *GoMap;
typedef void *GoChan;
typedef struct { void *t; void *v; } GoInterface;
typedef struct { void *data; int len; int cap; } GoSlice;


extern void lua_sandbox_print(void* p0, GoString p1);

extern void lua_sandbox_send_message(void* p0, GoString p1);
