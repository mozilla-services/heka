// +build geoip

package geoip

import (
	"bytes"
	"fmt"
	"github.com/abh/geoip"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"path/filepath"
	"strconv"
)

type GeoIpDecoderConfig struct {
        DatabaseFile string `toml:"db_file"`
        SourceIpField string `toml:"source_ip_field"`
        TargetField string `toml:"target_field"`

}

type GeoIpDecoder struct {
	DatabaseFile	string
	SourceIpField	string
	TargetField	string
	gi		*geoip.GeoIP
}

func (ld *GeoIpDecoder) ConfigStruct() interface{} {
	return &GeoIpDecoderConfig{
		DatabaseFile:   filepath.Join(Globals().ShareDir,"GeoLiteCity.dat"),
		SourceIpField:	"",
		TargetField:	"geoip",
	}
}

func (ld *GeoIpDecoder) Init(config interface{}) (err error) {
	conf := config.(*GeoIpDecoderConfig)


        if len(conf.DatabaseFile) > 0 {
                ld.DatabaseFile = conf.DatabaseFile
	}
	
	if len(conf.SourceIpField) > 0 {
		ld.SourceIpField = conf.SourceIpField
	} else {
		err = fmt.Errorf("Missing required flag: source_ip_field", err)
	}

	if len(conf.TargetField) > 0 {
                ld.TargetField = conf.TargetField
	} else {
		err = fmt.Errorf("Missing required flag: target_field", err)
	}

	return
}

func (ld *GeoIpDecoder) GetRecord(ip string) *geoip.GeoIPRecord {
                rec := ld.gi.GetRecord(ip)
                return rec
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
	defer func() { 
		if r := recover(); r != nil {
		   return 
	        }
	}()

	buf := bytes.Buffer{}
        file := ld.DatabaseFile
	ip := ""

	if len(ld.SourceIpField) > 0 {
		var ipAddr, _ = pack.Message.GetFieldValue(ld.SourceIpField)
		ip = ipAddr.(string)
	}

	if ld.gi == nil {
	        ld.gi, err = geoip.Open(file)
	}
        if err != nil {
                fmt.Printf("Could not open GeoIP database: %s\n", err)
        }
        if ld.gi != nil {
		rec := ld.gi.GetRecord(ip)
                buf = ld.GeoBuff(rec)		
        }

	if len(ld.TargetField) > 0 {
		if buf.Len() > 0  {
			nf, _ := message.NewField(ld.TargetField, buf.Bytes(), "")
			pack.Message.AddField(nf)
		} 
	} 
	packs = []*PipelinePack{pack}
	
    return
}

func init() {
        RegisterPlugin("GeoIpDecoder", func() interface{} {
                return new(GeoIpDecoder)
        })
}

