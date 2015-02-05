/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package process

import "time"

// ProcessChain test configuration
const SINGLE_CMD = "cat"

var SINGLE_CMD_ARGS = []string{"./testsupport/process_input_pipes_test.data"}

const SINGLE_CMD_OUTPUT = "this|is|a|test|\nignore this line\nand this line\n"

const NONZERO_TIMEOUT_CMD = "tail"

const NONZERO_TIMEOUT = time.Millisecond * 100

var NONZERO_TIMEOUT_ARGS = []string{"-f", "./testsupport/process_input_pipes_test.data"}

const STDERR_CMD = "tail"

var STDERR_CMD_ARGS = []string{"not_a_file.data"}

const PIPE_CMD1 = "cat"
const PIPE_CMD2 = "grep"

var PIPE_CMD1_ARGS = []string{"./testsupport/process_input_pipes_test.data"}
var PIPE_CMD2_ARGS = []string{"-i", "TEST"}

const PIPE_CMD_OUTPUT = "this|is|a|test|\n"

var TIMEOUT_PIPE_CMD1 = "tail"
var TIMEOUT_PIPE_CMD2 = "grep"
var TIMEOUT_PIPE_CMD1_ARGS = []string{"-f", "./testsupport/process_input_pipes_test.data"}
var TIMEOUT_PIPE_CMD2_ARGS = []string{"-i", "TEST"}

// ProcessInput test configuration
const PROCESSINPUT_TEST1_CMD = "cat"

var PROCESSINPUT_TEST1_CMD_ARGS = []string{"./testsupport/process_input_test.data"}
var PROCESSINPUT_TEST1_OUTPUT = "this|is|a|test|"

const PROCESSINPUT_PIPE_CMD1 = "cat"

var PROCESSINPUT_PIPE_CMD1_ARGS = []string{"./testsupport/process_input_pipes_test.data"}

const PROCESSINPUT_PIPE_CMD2 = "grep"

var PROCESSINPUT_PIPE_CMD2_ARGS = []string{"ignore"}
var PROCESSINPUT_PIPE_OUTPUT = "ignore this line"
