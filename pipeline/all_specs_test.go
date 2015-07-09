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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"github.com/rafrombrc/gospec/src/gospec"
	"testing"
)

func mockDecoderCreator() map[string]Decoder {
	return make(map[string]Decoder)
}

func mockFilterCreator() map[string]Filter {
	return make(map[string]Filter)
}

func mockOutputCreator() map[string]Output {
	return make(map[string]Output)
}

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.Parallel = false

	r.AddSpec(BufferedOutputSpec)
	r.AddSpec(InputRunnerSpec)
	r.AddSpec(OutputRunnerSpec)
	r.AddSpec(SplitterRunnerSpec)
	r.AddSpec(MessageTemplateSpec)
	r.AddSpec(ProtobufDecoderSpec)
	r.AddSpec(ReportSpec)
	r.AddSpec(StatAccumInputSpec)
	r.AddSpec(TokenSpec)
	r.AddSpec(RegexSpec)
	r.AddSpec(HekaFramingSpec)

	gospec.MainGoTest(r, t)
}

func BenchmarkPipelinePackCreation(b *testing.B) {
	var config = PipelineConfig{}
	for i := 0; i < b.N; i++ {
		NewPipelinePack(config.inputRecycleChan)
	}
}
