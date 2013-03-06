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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import "github.com/mozilla-services/heka/message"

type metric struct {
	Type_ string `json:"type"`
	Name  string
	Value string
}

type StatFilterConfig struct {
	Match  MatchCriteriaLayout
	Metric []metric
}

type StatFilter struct {
	msgMatcher MessageMatcher
	metrics    []metric
}

func (s *StatFilter) ConfigStruct() interface{} {
	return new(StatFilterConfig)
}

func (s *StatFilter) Init(config interface{}) (err error) {
	conf := config.(*StatFilterConfig)
	if s.msgMatcher, err = NewMessageMatcher(&conf.Match); err != nil {
		return
	}
	s.metrics = conf.Metric
	return
}

//func (s *StatFilter) FilterMsg(pack *PipelinePack) {
//	set, matched := s.msgMatcher.Match(pack.Message)
//	if !matched {
//		return
//	}
//
//	// Load existing fields into the set for replacement
//	set["Logger"] = pack.Message.GetLogger()
//	set["Hostname"] = pack.Message.GetHostname()
//	set["Type"] = pack.Message.GetType()
//	set["Payload"] = pack.Message.GetPayload()
//
//	// We matched, generate appropriate metrics
//	for _, m := range s.metrics {
//		msg := MessageGenerator.Retrieve()
//		msg.Message.SetType(m.Type_)
//		msg.Message.SetLogger(InterpolateString(m.Name, set))
//		msg.Message.SetPayload(InterpolateString(m.Value, set))
//		MessageGenerator.Inject(msg)
//	}
//	return
//}

func (s *StatFilter) ProcessMessage(m *message.Message) int {
	set, matched := s.msgMatcher.Match(m)
	if !matched {
		return 0
	}

	// Load existing fields into the set for replacement
	set["Logger"] = m.GetLogger()
	set["Hostname"] = m.GetHostname()
	set["Type"] = m.GetType()
	set["Payload"] = m.GetPayload()

	// We matched, generate appropriate metrics
	for _, m := range s.metrics {
		msg := MessageGenerator.Retrieve()
		msg.Message.SetType(m.Type_)
		msg.Message.SetLogger(InterpolateString(m.Name, set))
		msg.Message.SetPayload(InterpolateString(m.Value, set))
		MessageGenerator.Inject(msg)
	}
	return 0
}

func (s *StatFilter) SetOutput(f func(s string)) {
}

func (s *StatFilter) SetInjectMessage(f func(s string)) {
}

func (s *StatFilter) TimerEvent() int {
	return 0
}

func (s *StatFilter) Destroy() {
}
