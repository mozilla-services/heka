.. _tls:

===============
Configuring TLS
===============

Many input and output plugins that rely on TCP as the underlying transport for
network communication also support the use of SSL/TLS encryption for their
connections. Typically the TOML configuration for these plugins will support a
boolean `use_tls` flag that specifies whether or not encryption should be
used, and a `tls` sub-section that specifies the settings to be used for
negotiating the TLS connections. If `use_tls` is not set to true, the `tls`
section will be ignored.

Modeled after Go's stdlib TLS `configuration struct
<http://golang.org/pkg/crypto/tls/#Config>`_, the same configuration
structure is used for both client and server connections, with some of the
settings being applicable for a client's configuration, some for a server's,
and some for both. In the description of the TLS configuration settings below,
each setting is marked as appropriate to client, server, or both as
appropriate.

TLS configuration settings
==========================

- server_name (string, client):
	Name of the server being requested. Included in the client handshake to
	support virtual hosting server environments.
- cert_file (string, both):
    Full filesystem path to the certificate file to be presented to the other
    side of the connection.
- key_file (string, both):
    Full filesystem path to the specified certificate's associated private key
    file.
- client_auth  (string, server):
	Specifies the server's policy for TLS client authentication. Must be one
	of the following values:
	
	- NoClientCert
	- RequestClientCert
	- RequireAnyClientCert
	- VerifyClientCertIfGiven
	- RequireAndVerifyClientCert
	
	Defaults to "NoClientCert".
- ciphers ([]string, both):
	List of cipher suites supported for TLS connections. Earlier suites in the
	list have priority over those following. Must only contain values from the
	following selection:

	- RSA_WITH_RC4_128_SHA
	- RSA_WITH_3DES_EDE_CBC_SHA
	- RSA_WITH_AES_128_CBC_SHA
	- RSA_WITH_AES_256_CBC_SHA
	- ECDHE_ECDSA_WITH_RC4_128_SHA
	- ECDHE_ECDSA_WITH_AES_128_CBC_SHA
	- ECDHE_ECDSA_WITH_AES_256_CBC_SHA
	- ECDHE_RSA_WITH_RC4_128_SHA
	- ECDHE_RSA_WITH_3DES_EDE_CBC_SHA
	- ECDHE_RSA_WITH_AES_128_CBC_SHA
	- ECDHE_RSA_WITH_AES_256_CBC_SHA
	- ECDHE_RSA_WITH_AES_128_GCM_SHA256
	- ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	
	If omitted, the implementation's `default ordering
	<http://golang.org/src/pkg/crypto/tls/cipher_suites.go#L69>`_ will be
	used.
- insecure_skip_verify (bool, client):
	If true, TLS client connections will accept any certificate presented by
	the server and any host name in that certificate. This causes TLS to be
	susceptible to man-in-the-middle attacks and should only be used for
	testing. Defaults to false.
- prefer_server_ciphers (bool, server):
	If true, a server will always favor the server's specified cipher suite
	priority order over that requested by the client. Defaults to true.
- session_tickets_disabled (bool, server):
	If true, session resumption support as specified in `RFC 5077
	<http://tools.ietf.org/search/rfc5077>`_ will be disabled.
- session_ticket_key (string, server):
	Used by the TLS server to provide session resumption per `RFC 5077
	<http://tools.ietf.org/search/rfc5077>`_. If left empty, it will be filled
	with random data before the first server handshake.
- min_version (string, both):
	Specifies the mininum acceptable SSL/TLS version. Must
	be one of the following values:
	
	- SSL30
	- TLS10
	- TLS11
	- TLS12

	Defaults to SSL30.

- max_version (string, both):
	Specifies the maximum acceptable SSL/TLS version. Must
	be one of the following values:

	- SSL30
	- TLS10
	- TLS11
	- TLS12

	Defaults to TLS12.

- client_cafile (string, server):
	File for server to authenticate client TLS handshake. Any client certs recieved by server
	must be chained to a CA found in this PEM file.
    
	Has no effect when NoClientCert is set.

- root_cafile (string, client):
	File for client to authenticate server TLS handshake. Any server certs recieved by client
	must be must be chained to a CA found in this PEM file.

Sample TLS configuration
========================

The following is a sample TcpInput configuration showing the use of TLS
encryption.

.. code-block:: ini

    [TcpInput]
    address = ":5565"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"
    use_tls = true

        [TcpInput.tls]
        cert_file = "/usr/share/heka/tls/cert.pem"
        key_file = "/usr/share/heka/tls/cert.key"
        client_auth = "RequireAndVerifyClientCert"
        prefer_server_ciphers = true
        min_version = "TLS11"
