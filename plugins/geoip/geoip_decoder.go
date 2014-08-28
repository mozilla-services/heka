// +build geoip

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
	"bytes"
	"errors"
	"fmt"
	"github.com/abh/geoip"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"strconv"
)

type GeoIpDecoderConfig struct {
	DatabaseFile  string `toml:"db_file"`
	SourceIpField string `toml:"source_ip_field"`
	TargetField   string `toml:"target_field"`
}

type GeoIpDecoder struct {
	DatabaseFile  string
	SourceIpField string
	TargetField   string
	gi            *geoip.GeoIP
	pConfig       *PipelineConfig
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (ld *GeoIpDecoder) SetPipelineConfig(pConfig *PipelineConfig) {
	ld.pConfig = pConfig
}

func (ld *GeoIpDecoder) ConfigStruct() interface{} {
	globals := ld.pConfig.Globals
	return &GeoIpDecoderConfig{
		DatabaseFile:  globals.PrependShareDir("GeoLiteCity.dat"),
		SourceIpField: "",
		TargetField:   "geoip",
	}
}

func (ld *GeoIpDecoder) Init(config interface{}) (err error) {
	conf := config.(*GeoIpDecoderConfig)

	if string(conf.SourceIpField) == "" {
		return errors.New("`source_ip_field` must be specified")
	}

	if conf.TargetField == "" {
		return errors.New("`target_field` must be specified")
	}

	ld.TargetField = conf.TargetField
	ld.SourceIpField = conf.SourceIpField

	if ld.gi == nil {
		ld.gi, err = geoip.Open(conf.DatabaseFile)
	}
	if err != nil {
		return fmt.Errorf("Could not open GeoIP database: %s\n")
	}

	return
}

func (ld *GeoIpDecoder) GetRecord(ip string) *geoip.GeoIPRecord {
	return ld.gi.GetRecord(ip)
}

func (ld *GeoIpDecoder) GeoBuff(rec *geoip.GeoIPRecord) bytes.Buffer {
	buf := bytes.Buffer{}

	latitudeString := strconv.FormatFloat(float64(rec.Latitude), 'g', 16, 32)
	longitudeString := strconv.FormatFloat(float64(rec.Longitude), 'g', 16, 32)
	areacodeString := strconv.FormatInt(int64(rec.AreaCode), 10)
	charsetString := strconv.FormatInt(int64(rec.CharSet), 10)

	buf.WriteString(`{`)

	buf.WriteString(`"latitude":`)
	buf.WriteString(latitudeString)

	buf.WriteString(`,"longitude":`)
	buf.WriteString(longitudeString)

	buf.WriteString(`,"location":[`)
	buf.WriteString(longitudeString)
	buf.WriteString(`,`)
	buf.WriteString(latitudeString)
	buf.WriteString(`]`)

	buf.WriteString(`,"coordinates":["`)
	buf.WriteString(longitudeString)
	buf.WriteString(`","`)
	buf.WriteString(latitudeString)
	buf.WriteString(`"]`)

	buf.WriteString(`,"countrycode":"`)
	buf.WriteString(rec.CountryCode)
	buf.WriteString(`"`)

	buf.WriteString(`,"countrycode3":"`)
	buf.WriteString(rec.CountryCode3)
	buf.WriteString(`"`)

	buf.WriteString(`,"countryname":"`)
	buf.WriteString(rec.CountryName)
	buf.WriteString(`"`)

	buf.WriteString(`,"region":"`)
	buf.WriteString(rec.Region)
	buf.WriteString(`"`)

	buf.WriteString(`,"city":"`)
	buf.WriteString(rec.City)
	buf.WriteString(`"`)

	buf.WriteString(`,"postalcode":"`)
	buf.WriteString(rec.PostalCode)
	buf.WriteString(`"`)

	buf.WriteString(`,"areacode":`)
	buf.WriteString(areacodeString)

	buf.WriteString(`,"charset":`)
	buf.WriteString(charsetString)

	buf.WriteString(`,"continentcode":"`)
	buf.WriteString(rec.ContinentCode)
	buf.WriteString(`"`)

	buf.WriteString(`}`)

	return buf
}

func (ld *GeoIpDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	var buf bytes.Buffer
	var ipAddr, _ = pack.Message.GetFieldValue(ld.SourceIpField)

	ip, ok := ipAddr.(string)

	if !ok {
		// IP field was not a string. Field could just be blank. Return without error.
		packs = []*PipelinePack{pack}
		return
	}

	if ld.gi != nil {
		rec := ld.gi.GetRecord(ip)
		if rec != nil {
			buf = ld.GeoBuff(rec)
		} else {
			// IP address did not return a valid GeoIp record but that's ok sometimes(private ip?). Return without error.
			packs = []*PipelinePack{pack}
			return
		}
	}

	if buf.Len() > 0 {
		var nf *message.Field
		nf, err = message.NewField(ld.TargetField, buf.Bytes(), "")
		pack.Message.AddField(nf)
	}

	packs = []*PipelinePack{pack}

	return
}

func init() {
	RegisterPlugin("GeoIpDecoder", func() interface{} {
		return new(GeoIpDecoder)
	})
}
