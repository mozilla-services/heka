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
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	. "github.com/mozilla-services/heka/message"
	"log"
)

// Represents message lookup hashes
//
// ChainMap is populated such that a message type should exactly
// match and return a list representing keys in FilterChains. At the
// moment the list will always be a single element, but in the future
// with more ways to restrict the filter chain to other components of
// the message narrowing down the set for several will be needed.
type ChainRouter struct {
	InChan   chan *PipelinePack
	ChainMap map[string][]string
}

func (self *ChainRouter) Start() {
	var pack *PipelinePack
	var i int
	go func() {
		for {
			pack = <-self.InChan
			pack.OutputNames = map[string]bool{}
			chainName, ok := self.LocateChain(pack.Message)
			if ok {
				pack.FilterChain = chainName
			} else {
				chainName = pack.FilterChain
			}
			chain, ok := pack.Config.FilterChains[chainName]
			if !ok {
				log.Printf("Filter chain doesn't exist: %s", chainName)
				pack.Recycle()
				continue
			}
			for _, outputName := range chain.Outputs {
				pack.OutputNames[outputName] = true
			}
			for _, filterName := range chain.Filters {
				filter := pack.Filters[filterName]
				filter.FilterMsg(pack)
				if pack.Blocked {
					pack.Recycle()
					continue
				}
			}

			i = 0
			for outputName, use := range pack.OutputNames {
				if !use {
					continue
				}
				outChan, ok := pack.OutputChans[outputName]
				if !ok {
					log.Printf("Output doesn't exist: %s\n", outputName)
					continue
				}
				outChan <- pack
				i++
			}
			if i == 0 {
				pack.Recycle()
			}
		}
	}()
}

func (self *ChainRouter) LocateChain(message *Message) (string, bool) {
	if chains, ok := self.ChainMap[message.GetType()]; ok {
		return chains[0], true
	}
	return "", false
}
