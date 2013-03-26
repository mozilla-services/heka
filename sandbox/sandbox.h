/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// Common sandbox configuration for Heka plugins @file
#ifndef sandbox_h_
#define sandbox_h_

#ifdef _WIN32
    #if defined(sandbox_EXPORTS)
        #define SANDBOX_EXPORT __declspec(dllexport)
    #else
        #define SANDBOX_EXPORT __declspec(dllimport)
    #endif
#else
    #define SANDBOX_EXPORT
#endif

typedef enum {
    STATUS_UNKNOWN      = 0,
    STATUS_RUNNING      = 1,
    STATUS_TERMINATED   = 2
} sandbox_status;

typedef enum {
    US_LIM   = 0,
    US_CUR   = 1,
    US_MAX   = 2,

    MAX_USAGE_STAT
} sandbox_usage_stat;

typedef enum {
    UT_MEM  = 0,
    UT_INS  = 1,
    UT_OUT  = 2,

    MAX_USAGE_TYPE
} sandbox_usage_type;

#endif
