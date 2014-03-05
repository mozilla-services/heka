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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
	"crypto/tls"
	"encoding/hex"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"strings"
)

func TlsSpec(c gs.Context) {
	tomlConf := new(TlsConfig)
	var goConf *tls.Config
	var err error

	c.Specify("The CreateGoTlsConfig function", func() {
		c.Specify("creates a client tls.config struct", func() {
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(goConf.ServerName, gs.Equals, "")
		})

		c.Specify("creates a server tls.config struct", func() {
			tomlConf.ServerName = "server.name"
			tomlConf.CertFile = "./testsupport/cert.pem"
			tomlConf.KeyFile = "./testsupport/key.pem"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(goConf.ServerName, gs.Equals, tomlConf.ServerName)
			c.Expect(len(goConf.Certificates), gs.Equals, 1)
		})

		c.Specify("works when key and cert are in the same file", func() {
			tomlConf.ServerName = "server.name"
			tomlConf.CertFile = "./testsupport/both.pem"
			tomlConf.KeyFile = "./testsupport/both.pem"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(goConf.ServerName, gs.Equals, tomlConf.ServerName)
			c.Expect(len(goConf.Certificates), gs.Equals, 1)
		})

		c.Specify("fails when max version is less than min version", func() {
			tomlConf.MaxVersion = "TLS10"
			tomlConf.MinVersion = "TLS11"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(strings.Contains(err.Error(), "must be newer"), gs.IsTrue)
			c.Expect(goConf, gs.IsNil)
		})

		c.Specify("works when min version is specified w/ no max", func() {
			tomlConf.MinVersion = "TLS11"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(goConf.MinVersion, gs.Equals, uint16(tls.VersionTLS11))
			c.Expect(goConf.MaxVersion, gs.Equals, uint16(0))
		})

		c.Specify("fails w/ invalid ClientAuth", func() {
			tomlConf.ClientAuth = "foo"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "Invalid ClientAuth value: foo")
		})

		c.Specify("works w/ valid SessionTicketKey length", func() {
			tomlConf.SessionTicketKey = "1234567890123456789012345678901234567890123456789012345678901234"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			compareKey := hex.EncodeToString(goConf.SessionTicketKey[:32])
			c.Expect(compareKey, gs.Equals, tomlConf.SessionTicketKey)
		})

		c.Specify("fails w/ invalid SessionTicketKey length", func() {
			tomlConf.SessionTicketKey = "12345678901234567890123456789012345678901234567890123456789012"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "Invalid SessionTicketKey length: 31")
		})

		c.Specify("works w/ valid cipher strings", func() {
			tomlConf.Ciphers = []string{"RSA_WITH_AES_256_CBC_SHA",
				"ECDHE_ECDSA_WITH_AES_256_CBC_SHA"}
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(len(goConf.CipherSuites), gs.Equals, 2)
			c.Expect(goConf.CipherSuites[0], gs.Equals, tls.TLS_RSA_WITH_AES_256_CBC_SHA)
			c.Expect(goConf.CipherSuites[1], gs.Equals, tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA)
		})

		c.Specify("fails w/ invalid cipher string", func() {
			tomlConf.Ciphers = []string{"foo"}
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "Invalid cipher string: foo")
		})

		c.Specify("root_cafile loads CA", func() {
			tomlConf.ClientCAs = "./testsupport/cert.pem"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(len(goConf.ClientCAs.Subjects()), gs.Equals, 1)
		})

		c.Specify("client_cafile loads CA", func() {
			tomlConf.RootCAs = "./testsupport/cert.pem"
			goConf, err = CreateGoTlsConfig(tomlConf)
			c.Expect(err, gs.IsNil)
			c.Expect(len(goConf.RootCAs.Subjects()), gs.Equals, 1)
		})

	})
}
