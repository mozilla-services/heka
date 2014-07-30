/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Michael Gibson (michael.gibson79@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package geoip

import (
	"github.com/abh/geoip"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"testing"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(GeoIpDecoderSpec)

	gs.MainGoTest(r, t)
}

func GeoIpDecoderSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pConfig := NewPipelineConfig(nil)
	pConfig.Globals.ShareDir = "/foo/bar/baz"

	c.Specify("A GeoIpDecoder", func() {
		decoder := new(GeoIpDecoder)
		decoder.SetPipelineConfig(pConfig)
		rec := new(geoip.GeoIPRecord)
		conf := decoder.ConfigStruct().(*GeoIpDecoderConfig)

		c.Expect(conf.DatabaseFile, gs.Equals, "/foo/bar/baz/GeoLiteCity.dat")

		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		nf, _ := message.NewField("remote_host", "74.125.142.147", "")
		pack.Message.AddField(nf)

		decoder.SourceIpField = "remote_host"
		conf.SourceIpField = "remote_host"
		decoder.Init(conf)

		rec.CountryCode = "US"
		rec.CountryCode3 = "USA"
		rec.CountryName = "United States"
		rec.Region = "CA"
		rec.City = "Mountain View"
		rec.PostalCode = "94043"
		rec.Latitude = 37.4192
		rec.Longitude = -122.0574
		rec.AreaCode = 650
		rec.CharSet = 1
		rec.ContinentCode = "NA"

		c.Specify("Test GeoIpDecoder Output", func() {
			buf := decoder.GeoBuff(rec)
			nf, _ = message.NewField("geoip", buf.Bytes(), "")
			pack.Message.AddField(nf)

			b, ok := pack.Message.GetFieldValue("geoip")
			c.Expect(ok, gs.IsTrue)

			c.Expect(string(b.([]byte)), gs.Equals, `{"latitude":37.4192008972168,"longitude":-122.0574035644531,"location":[-122.0574035644531,37.4192008972168],"coordinates":["-122.0574035644531","37.4192008972168"],"countrycode":"US","countrycode3":"USA","countryname":"United States","region":"CA","city":"Mountain View","postalcode":"94043","areacode":650,"charset":1,"continentcode":"NA"}`)
		})

	})
}
