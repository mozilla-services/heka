/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
)

// Decoder for converting ProtocolBuffer data into Message objects.
type ProtobufDecoder struct {
	processMessageCount    int64
	processMessageFailures int64
	processMessageSamples  int64
	processMessageDuration int64
	pConfig                *PipelineConfig
	reportLock             sync.Mutex
	sample                 bool
	sampleDenominator      int
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (p *ProtobufDecoder) SetPipelineConfig(pConfig *PipelineConfig) {
	p.pConfig = pConfig
}

func (p *ProtobufDecoder) Init(config interface{}) error {
	p.sample = true
	p.sampleDenominator = p.pConfig.Globals.SampleDenominator
	return nil
}

func (p *ProtobufDecoder) Decode(pack *PipelinePack) (
	packs []*PipelinePack, err error) {

	atomic.AddInt64(&p.processMessageCount, 1)

	var startTime time.Time
	if p.sample {
		startTime = time.Now()
	}

	if err = proto.Unmarshal(pack.MsgBytes, pack.Message); err == nil {
		packs = []*PipelinePack{pack}
		pack.TrustMsgBytes = true
	} else {
		atomic.AddInt64(&p.processMessageFailures, 1)
	}

	if p.sample {
		duration := time.Since(startTime).Nanoseconds()
		p.reportLock.Lock()
		p.processMessageDuration += duration
		p.processMessageSamples++
		p.reportLock.Unlock()
	}
	p.sample = 0 == rand.Intn(p.sampleDenominator)
	return
}

func (p *ProtobufDecoder) EncodesMsgBytes() bool {
	return true
}

func (p *ProtobufDecoder) ReportMsg(msg *message.Message) error {
	p.reportLock.Lock()
	defer p.reportLock.Unlock()

	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&p.processMessageCount), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures",
		atomic.LoadInt64(&p.processMessageFailures), "count")
	message.NewInt64Field(msg, "ProcessMessageSamples",
		p.processMessageSamples, "count")

	var tmp int64 = 0
	if p.processMessageSamples > 0 {
		tmp = p.processMessageDuration / p.processMessageSamples
	}
	message.NewInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	return nil
}

// Encoder for converting Message objects into Protocol Buffer data.
type ProtobufEncoder struct {
	processMessageCount    int64
	processMessageFailures int64
	processMessageSamples  int64
	processMessageDuration int64
	pConfig                *PipelineConfig
	reportLock             sync.Mutex
	sample                 bool
	sampleDenominator      int
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (p *ProtobufEncoder) SetPipelineConfig(pConfig *PipelineConfig) {
	p.pConfig = pConfig
}

func (p *ProtobufEncoder) Init(config interface{}) error {
	p.sample = true
	p.sampleDenominator = p.pConfig.Globals.SampleDenominator
	return nil
}

func (p *ProtobufEncoder) Encode(pack *PipelinePack) (output []byte, err error) {
	atomic.AddInt64(&p.processMessageCount, 1)
	var startTime time.Time
	if p.sample {
		startTime = time.Now()
	}

	// Once the reimplementation of the output API is finished we should be
	// able to just return pack.MsgBytes directly, but for now we need to copy
	// the data to prevent problems in case the pack is zeroed and/or reused
	// (overwriting the pack.MsgBytes memory) before we're done with it.
	output = make([]byte, len(pack.MsgBytes))
	copy(output, pack.MsgBytes)

	if p.sample {
		duration := time.Since(startTime).Nanoseconds()
		p.reportLock.Lock()
		p.processMessageDuration += duration
		p.processMessageSamples++
		p.reportLock.Unlock()
	}
	p.sample = 0 == rand.Intn(p.sampleDenominator)
	return output, nil
}

func (p *ProtobufEncoder) Stop() {
	return
}

func (p *ProtobufEncoder) ReportMsg(msg *message.Message) error {
	p.reportLock.Lock()
	defer p.reportLock.Unlock()

	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&p.processMessageCount), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures",
		atomic.LoadInt64(&p.processMessageFailures), "count")
	message.NewInt64Field(msg, "ProcessMessageSamples",
		p.processMessageSamples, "count")

	var tmp int64 = 0
	if p.processMessageSamples > 0 {
		tmp = p.processMessageDuration / p.processMessageSamples
	}
	message.NewInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	return nil
}

func init() {
	RegisterPlugin("ProtobufDecoder", func() interface{} {
		return new(ProtobufDecoder)
	})
	RegisterPlugin("ProtobufEncoder", func() interface{} {
		return new(ProtobufEncoder)
	})
}
