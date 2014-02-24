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
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io/ioutil"
)

var ciphers map[string]uint16 = map[string]uint16{
	"RSA_WITH_RC4_128_SHA":                tls.TLS_RSA_WITH_RC4_128_SHA,
	"RSA_WITH_3DES_EDE_CBC_SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE_ECDSA_WITH_RC4_128_SHA":        tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	"ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"ECDHE_RSA_WITH_RC4_128_SHA":          tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	"ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
}

var tlsVersions map[string]uint16 = map[string]uint16{
	"SSL30": tls.VersionSSL30,
	"TLS10": tls.VersionTLS10,
	"TLS11": tls.VersionTLS11,
	"TLS12": tls.VersionTLS12,
}

var clientAuthTypes map[string]tls.ClientAuthType = map[string]tls.ClientAuthType{
	"NoClientCert":               tls.NoClientCert,
	"RequestClientCert":          tls.RequestClientCert,
	"RequireAnyClientCert":       tls.RequireAnyClientCert,
	"VerifyClientCertIfGiven":    tls.VerifyClientCertIfGiven,
	"RequireAndVerifyClientCert": tls.RequireAndVerifyClientCert,
}

type TlsConfig struct {
	// Paths to certificate files, keyed by certificate name.
	ServerName             string `toml:"server_name"`
	CertFile               string `toml:"cert_file"`
	KeyFile                string `toml:"key_file"`
	ClientAuth             string `toml:"client_auth"`
	Ciphers                []string
	InsecureSkipVerify     bool   `toml:"insecure_skip_verify"`
	PreferServerCiphers    bool   `toml:"prefer_server_ciphers"`
	SessionTicketsDisabled bool   `toml:"session_tickets_disabled"`
	SessionTicketKey       string `toml:"sesion_ticket_key"`
	MinVersion             string `toml:"min_version"`
	MaxVersion             string `toml:"max_version"`
	ClientCAs              string `toml:"client_cafile"`
	RootCAs                string `toml:"root_cafile"`
}

func CreateGoTlsConfig(tomlConf *TlsConfig) (goConf *tls.Config, err error) {
	goConf = new(tls.Config)

	if tomlConf.CertFile != "" && tomlConf.KeyFile != "" {
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(tomlConf.CertFile, tomlConf.KeyFile)
		if err != nil {
			return
		}
		goConf.Certificates = []tls.Certificate{cert}
		goConf.NameToCertificate = make(map[string]*tls.Certificate)
		goConf.NameToCertificate["default"] = &cert
	}
	goConf.ServerName = tomlConf.ServerName

	var ok bool
	if tomlConf.MinVersion != "" {
		goConf.MinVersion, ok = tlsVersions[tomlConf.MinVersion]
		if !ok {
			return nil, fmt.Errorf("Invalid MinVersion: %s", tomlConf.MinVersion)
		}
	}
	if tomlConf.MaxVersion != "" {
		goConf.MaxVersion, ok = tlsVersions[tomlConf.MaxVersion]
		if !ok {
			return nil, fmt.Errorf("Invalid MaxVersion: %s", tomlConf.MaxVersion)
		}
	}
	if goConf.MaxVersion > 0 && goConf.MaxVersion < goConf.MinVersion {
		return nil, fmt.Errorf("MaxVersion (%s) must be newer than MinVersion (%s)",
			tomlConf.MaxVersion, tomlConf.MinVersion)
	}

	if tomlConf.RootCAs != "" {
		if goConf.RootCAs, err = certPoolFromFile(tomlConf.RootCAs); err != nil {
			return nil, err
		}
	}

	if tomlConf.ClientCAs != "" {
		if goConf.ClientCAs, err = certPoolFromFile(tomlConf.ClientCAs); err != nil {
			return nil, err
		}
	}

	if tomlConf.ClientAuth != "" {
		goConf.ClientAuth, ok = clientAuthTypes[tomlConf.ClientAuth]
		if !ok {
			return nil, fmt.Errorf("Invalid ClientAuth value: %s", tomlConf.ClientAuth)
		}
	}

	goConf.InsecureSkipVerify = tomlConf.InsecureSkipVerify
	goConf.PreferServerCipherSuites = tomlConf.PreferServerCiphers
	goConf.SessionTicketsDisabled = tomlConf.SessionTicketsDisabled

	if tomlConf.SessionTicketKey != "" {
		var ticketKey []byte
		if ticketKey, err = hex.DecodeString(tomlConf.SessionTicketKey); err != nil {
			return nil, err
		}
		if len(ticketKey) != 32 {
			return nil, fmt.Errorf("Invalid SessionTicketKey length: %d",
				len(ticketKey))
		}
		copy(goConf.SessionTicketKey[:32], ticketKey[:])
	}

	var cipher uint16
	for _, cipherStr := range tomlConf.Ciphers {
		if cipher, ok = ciphers[cipherStr]; !ok {
			return nil, fmt.Errorf("Invalid cipher string: %s", cipherStr)
		}
		goConf.CipherSuites = append(goConf.CipherSuites, cipher)
	}
	return
}

func certPoolFromFile(pemfile string) (*x509.CertPool, error) {
	roots := x509.NewCertPool()
	data, err := ioutil.ReadFile(pemfile)
	if err != nil {
		return nil, err
	}
	if roots.AppendCertsFromPEM(data) {
		return roots, nil
	}
	return nil, fmt.Errorf("No PEM encoded certificates found in: %s\n", pemfile)
}
