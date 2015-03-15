/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#   Justin Judd (justin@justinjudd.org)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// multiDecoderNode is used for making sure MultiDecoder dependencies are addressed
type multiDecoderNode struct {
	name            string
	subs            []string
	dependencyCount int
}

func newMultiDecoderNode(name string, subs []string) multiDecoderNode {
	return multiDecoderNode{name, subs, -1}
}

// multiDecoderNodeList fulfills sort.Interface
type multiDecoderNodeList []multiDecoderNode

func (m multiDecoderNodeList) Len() int {
	return len(m)
}

func (m multiDecoderNodeList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m multiDecoderNodeList) Less(i, j int) bool {
	a := m[i]
	b := m[j]

	return a.dependencyCount < b.dependencyCount
}

// utility function to make a copy of a map
func copyMap(m map[string]bool) map[string]bool {
	d := map[string]bool{}
	for k, v := range m {
		d[k] = v
	}
	return d
}

// orderDependencies determines MultiDecoder hierarchy and orders them accordingly
func orderDependencies(decoders []multiDecoderNode) ([]multiDecoderNode, error) {

	nodeMap := make(map[string]multiDecoderNode, 0)
	for _, node := range decoders {
		nodeMap[node.name] = node
	}

	// rankFunc is a recursive function to traverse graphs from nodes to the root
	var rankFunc func(name string, seen map[string]bool) (int, error)
	rankFunc = func(name string, seen map[string]bool) (int, error) {
		if seen[name] { // If this node has been seen in this path, it means that there is a circular dependency
			return -1, errors.New("circular dependency detected")
		}
		node := nodeMap[name]
		if node.dependencyCount >= 0 { // If dependency count is not -1, than we have already calculated it
			return node.dependencyCount, nil
		}
		dependencyCount := len(node.subs)
		seen[name] = true
		for _, sub := range node.subs {
			c, err := rankFunc(sub, copyMap(seen))
			if err != nil {
				return -1, err
			}
			dependencyCount += c
		}
		node.dependencyCount = dependencyCount
		nodeMap[name] = node
		return dependencyCount, nil
	}

	// Make sure that rankFunc is called for each multiDecoderNode
	for _, node := range decoders {
		_, err := rankFunc(node.name, map[string]bool{})
		if err != nil {
			return nil, err
		}
	}
	// Compile all nodes with dependency count into slice
	finishedNodes := make(multiDecoderNodeList, 0)
	for _, node := range nodeMap {
		finishedNodes = append(finishedNodes, node)
	}

	// sort nodes so that those with no dependencies are earlier in the list
	sort.Sort(finishedNodes)

	return finishedNodes, nil
}

// DecoderRunner wrapper that the MultiDecoder will hand to any subs that ask
// for one. Shadows some data and methods, but doesn't spin up any goroutines.
type mDRunner struct {
	*dRunner
	decoder Decoder
	name    string
	subName string
}

func (mdr *mDRunner) Name() string {
	return mdr.name
}

func (mdr *mDRunner) SetName(name string) {
	mdr.name = name
}

func (mdr *mDRunner) Plugin() Plugin {
	return mdr.decoder.(Plugin)
}

func (mdr *mDRunner) Decoder() Decoder {
	return mdr.decoder
}

func (mdr *mDRunner) LogError(err error) {
	LogError.Printf("SubDecoder '%s' error: %s", mdr.name, err)
}

func (mdr *mDRunner) LogMessage(msg string) {
	LogInfo.Printf("SubDecoder '%s': %s", mdr.name, msg)
}

type MultiDecoder struct {
	processMessageCount    []int64
	processMessageFailures []int64
	processMessageSamples  []int64
	processMessageDuration []int64
	totalMessageFailures   int64
	totalMessageSamples    int64
	totalMessageDuration   int64
	idx                    uint8
	sampleDenominator      int
	sample                 bool
	reportLock             sync.RWMutex
	pConfig                *PipelineConfig
	Config                 *MultiDecoderConfig
	Name                   string
	Decoders               []Decoder
	dRunner                DecoderRunner
	CascStrat              int
	neverTrustEncodes      bool
}

type MultiDecoderConfig struct {
	Subs            []string
	LogSubErrors    bool   `toml:"log_sub_errors"`
	CascadeStrategy string `toml:"cascade_strategy"`
}

const (
	CASC_FIRST_WINS = iota
	CASC_ALL
)

var mdStrategies = map[string]int{"first-wins": CASC_FIRST_WINS, "all": CASC_ALL}

func (md *MultiDecoder) ConfigStruct() interface{} {
	subs := make([]string, 0)
	return &MultiDecoderConfig{subs, false, "first-wins"}
}

// Heka will call this before calling Init() to set the name of the
// MultiDecoder based on the section name in the TOML config.
func (md *MultiDecoder) SetName(name string) {
	md.Name = name
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (md *MultiDecoder) SetPipelineConfig(pConfig *PipelineConfig) {
	md.pConfig = pConfig
}

func (md *MultiDecoder) Init(config interface{}) (err error) {
	md.Config = config.(*MultiDecoderConfig)

	numSubs := len(md.Config.Subs)
	if numSubs == 0 {
		return errors.New("At least one subdecoder must be specified.")
	}

	md.Decoders = make([]Decoder, len(md.Config.Subs))

	var (
		ok      bool
		decoder Decoder
	)

	if md.CascStrat, ok = mdStrategies[md.Config.CascadeStrategy]; !ok {
		return fmt.Errorf("Unrecognized cascade strategy: %s", md.Config.CascadeStrategy)
	}

	for i, name := range md.Config.Subs {
		if decoder, ok = md.pConfig.Decoder(name); !ok {
			return fmt.Errorf("Non-existent subdecoder: %s", name)
		}
		md.Decoders[i] = decoder
	}

	// We can trust the embedded decoders to leave the pack.MsgBytes and
	// pack.TrustMsgBytes values in the right state in all cases except when
	// cascade_strategy == "all", an earlier decoder sets the encoding, but
	// the last one in the list does not. We check for this case and, if so,
	// explicitly set pack.TrustMsgBytes to false for all packs on every
	// successful decode.
	if md.CascStrat == CASC_ALL {
		lastDecoder := md.Decoders[len(md.Decoders)-1]
		_, ok = lastDecoder.(EncodesMsgBytes)
		if !ok {
			for _, d := range md.Decoders {
				if _, ok = d.(EncodesMsgBytes); ok {
					md.neverTrustEncodes = true
					break
				}
			}
		}
	}

	md.processMessageCount = make([]int64, numSubs)
	md.processMessageFailures = make([]int64, numSubs)
	md.processMessageSamples = make([]int64, numSubs)
	md.processMessageDuration = make([]int64, numSubs)
	md.sampleDenominator = md.pConfig.Globals.SampleDenominator
	return nil
}

// Heka will call this to give us access to the runner. We'll store it for
// ourselves, but also have to pass on a wrapped version to any subdecoders
// that might need it.
func (md *MultiDecoder) SetDecoderRunner(dr DecoderRunner) {
	md.dRunner = dr
	for i, decoder := range md.Decoders {
		subName := md.Config.Subs[i]
		if wanter, ok := decoder.(WantsDecoderRunner); ok {
			// It wants a DecoderRunner, have to create one. But first we need
			// to get our hands on a *dRunner.
			var realDRunner *dRunner
			if realDRunner, ok = dr.(*dRunner); !ok {
				// It's not a *dRunner, maybe it's an *mDRunner?
				var mdr *mDRunner
				if mdr, ok = dr.(*mDRunner); ok {
					// Bingo, we can grab its *dRunner.
					realDRunner = mdr.dRunner
				}
			}
			if realDRunner == nil {
				// Couldn't get a *dRunner. Just log an error and pass the
				// outer DecoderRunner through.
				dr.LogError(fmt.Errorf("Can't create nested DecoderRunner for '%s'",
					subName))
				wanter.SetDecoderRunner(dr)
				continue
			}
			// We have a *dRunner, use it to create an *mDRunner.
			subRunner := &mDRunner{
				realDRunner,
				decoder,
				fmt.Sprintf("%s-%s", realDRunner.name, subName),
				subName,
			}
			wanter.SetDecoderRunner(subRunner)
		}
	}
}

// Heka will call this at DecoderRunner shutdown time, we might need to pass
// this along to subdecoders.
func (md *MultiDecoder) Shutdown() {
	for _, decoder := range md.Decoders {
		if wanter, ok := decoder.(WantsDecoderRunnerShutdown); ok {
			wanter.Shutdown()
		}
	}
}

// Recurses through a decoder chain, decoding the original pack and returning
// it and any generated extra packs.
func (md *MultiDecoder) getDecodedPacks(chain []Decoder, inPacks []*PipelinePack) (
	packs []*PipelinePack, anyMatch bool) {

	var startTime time.Time

	decoder := chain[0]
	for _, p := range inPacks {
		atomic.AddInt64(&md.processMessageCount[md.idx], 1)
		if md.sample {
			startTime = time.Now()
		}
		ps, err := decoder.Decode(p)
		if md.sample {
			duration := time.Since(startTime).Nanoseconds()
			md.reportLock.Lock()
			md.processMessageDuration[md.idx] += duration
			md.processMessageSamples[md.idx]++
			md.reportLock.Unlock()
		}
		if ps != nil {
			anyMatch = true
			packs = append(packs, ps...)
		} else {
			atomic.AddInt64(&md.processMessageFailures[md.idx], 1)
			if err != nil && md.Config.LogSubErrors {
				idx := len(md.Decoders) - len(chain)
				err = fmt.Errorf("Subdecoder '%s' decode error: %s",
					md.Config.Subs[idx], err)
				md.dRunner.LogError(err)
			}
			packs = append(packs, p)
		}
	}

	if len(chain) > 1 {
		md.idx++
		var otherMatch bool
		packs, otherMatch = md.getDecodedPacks(chain[1:], packs)
		anyMatch = anyMatch || otherMatch
	}

	return
}

// Runs the message payload against each of the decoders.
func (md *MultiDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	md.sample = (rand.Intn(md.sampleDenominator) == 0 ||
		atomic.LoadInt64(&md.processMessageCount[0]) == 0)

	var startTime time.Time
	if md.sample {
		startTime = time.Now()
		defer func() {
			duration := time.Since(startTime).Nanoseconds()
			md.reportLock.Lock()
			md.totalMessageDuration += duration
			md.totalMessageSamples++
			md.reportLock.Unlock()
		}()
	}

	if md.CascStrat == CASC_FIRST_WINS {
		var subStartTime time.Time

		for i, d := range md.Decoders {
			count := atomic.AddInt64(&md.processMessageCount[i], 1)
			if md.sample || count == 1 {
				subStartTime = time.Now()
			}
			packs, err = d.Decode(pack)
			if md.sample || count == 1 {
				duration := time.Since(subStartTime).Nanoseconds()
				md.reportLock.Lock()
				md.processMessageDuration[i] += duration
				md.processMessageSamples[i]++
				md.reportLock.Unlock()
			}
			if packs != nil {
				return
			}
			atomic.AddInt64(&md.processMessageFailures[i], 1)
			if err != nil && md.Config.LogSubErrors {
				err = fmt.Errorf("Subdecoder '%s' decode error: %s", md.Config.Subs[i],
					err)
				md.dRunner.LogError(err)
			}
		}
		// If we got this far none of the decoders succeeded.
		atomic.AddInt64(&md.totalMessageFailures, 1)
		err = errors.New("All subdecoders failed.")
		packs = nil
	} else {
		// If we get here we know cascade_strategy == "all".
		var anyMatch bool
		md.idx = 0
		packs, anyMatch = md.getDecodedPacks(md.Decoders, []*PipelinePack{pack})
		if !anyMatch {
			atomic.AddInt64(&md.totalMessageFailures, 1)
			err = errors.New("All subdecoders failed.")
			packs = nil
		} else if md.neverTrustEncodes {
			for _, p := range packs {
				p.TrustMsgBytes = false
			}
		}
	}
	return
}

func (md *MultiDecoder) EncodesMsgBytes() bool {
	return true
}

func (md *MultiDecoder) ReportMsg(msg *message.Message) error {
	md.reportLock.RLock()
	defer md.reportLock.RUnlock()

	var tmp int64
	for i, sub := range md.Config.Subs {
		message.NewInt64Field(msg,
			fmt.Sprintf("ProcessMessageCount-%s", sub),
			atomic.LoadInt64(&md.processMessageCount[i]), "count")

		message.NewInt64Field(msg,
			fmt.Sprintf("ProcessMessageFailures-%s", sub),
			atomic.LoadInt64(&md.processMessageFailures[i]), "count")

		message.NewInt64Field(msg,
			fmt.Sprintf("ProcessMessageSamples-%s", sub),
			md.processMessageSamples[i], "count")

		tmp = 0
		if md.processMessageSamples[i] > 0 {
			tmp = md.processMessageDuration[i] / md.processMessageSamples[i]
		}
		message.NewInt64Field(msg,
			fmt.Sprintf("ProcessMessageAvgDuration-%s", sub), tmp, "ns")
	}
	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&md.processMessageCount[0]), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures",
		atomic.LoadInt64(&md.totalMessageFailures), "count")
	message.NewInt64Field(msg, "ProcessMessageSamples", md.totalMessageSamples, "count")
	tmp = 0
	if md.totalMessageSamples > 0 {
		tmp = md.totalMessageDuration / md.totalMessageSamples
	}
	message.NewInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	return nil
}

func init() {
	RegisterPlugin("MultiDecoder", func() interface{} {
		return new(MultiDecoder)
	})
}
