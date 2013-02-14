/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"log"
	"sync/atomic"
)

// Represents message lookup hashes
type ChainRouter struct {
	InChan chan *PipelinePack
}

func (self *ChainRouter) Start() {
	go func() {
		for {
			pack := <-self.InChan
			for _, chain := range pack.Config.FilterChains {
				atomic.AddInt32(&pack.RefCount, 1)
				go processChain(chain, pack)
			}
			pack.Recycle()
		}
	}()
}

func processChain(chain *FilterChain, pack *PipelinePack) {

	defer pack.Recycle()
	if chain.MessageFilter != nil {
		if !chain.MessageFilter.IsMatch(pack.Message) {
			return
		}
	}

	for _, filterName := range chain.Filters {
		wrapper, ok := pack.Config.Filters[filterName]
		if !ok {
			log.Printf("Filter doesn't exist: %s\n", filterName)
			continue
		}
		filter := wrapper.Create().(Filter)
		filter.FilterMsg(pack)
	}

	for _, outputName := range chain.Outputs {
		outRunner, ok := pack.Config.OutputRunners[outputName]
		if !ok {
			log.Printf("Output doesn't exist: %s\n", outputName)
			continue
		}
		atomic.AddInt32(&pack.RefCount, 1)
		outRunner.InChan() <- pack
	}
}
