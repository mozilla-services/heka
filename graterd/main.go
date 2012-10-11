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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package main

import (
	"fmt"
	"heka/grater"
)

func main() {
	const udpAddr = "127.0.0.1:5565"
	udpInput := hekagrater.NewUdpInput(udpAddr)
	inputs := []hekagrater.Input{&udpInput}

	//logOutput := hekagrater.LogOutput{}
	counterOutput := hekagrater.NewCounterOutput()
	outputs := []hekagrater.Output{counterOutput}

	config := hekagrater.GraterConfig{Inputs: inputs, Outputs: outputs}
	fmt.Println("Starting UDP listener at: %s", udpAddr)
	hekagrater.Run(&config)
}
