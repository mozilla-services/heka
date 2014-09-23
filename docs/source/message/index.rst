.. _message:

============
Heka Message
============

Message Variables
=================
* uuid (required, []byte) - 16 byte array containing a type 4 UUID.
* timestamp (required, int64) - Number of nanoseconds since the UNIX epoch.
* type (optional, string) - Type of message i.e. "WebLog".
* logger (optional, string) - Data source i.e. "Apache", "TCPInput", "/var/log/test.log".
* severity (optional, int32) - `Syslog severity level. <http://en.wikipedia.org/wiki/Syslog#Severity_levels>`_
* payload (optional, string) - Textual data i.e. log line, filename.
* env_version (optional, string) - Envelope version.  Semantic version of the message content (http://semver.org/ (although in most cases it is just the major version)).
* pid (optional, int32) - Process ID that generated the message.
* hostname (optional, string) - Hostname that generated the message.
* fields (optional, Field) - Array of Field structures.

.. _field_variables:

Field Variables
===============
* name (required, string) - Name of the field (key).
* value_type (optional, int32) - Type of the value stored in this field.
    * STRING  = 0 (default)
    * BYTES   = 1
    * INTEGER = 2
    * DOUBLE  = 3
    * BOOL    = 4
* representation (optional, string) - Freeform metadata string where you can
  describe what the data in this field represents. This information 
  might provide cues to assist with processing, labeling, or rendering of the 
  data performed by downstream plugins or UI elements. Examples of common usage 
  follow: 

    * Numeric value representation - In most cases it is the `unit <http://en.wikipedia.org/wiki/International_System_of_Units>`_. 
        * count - It is a standard practice to use 'count' for raw values with no units.
        * KiB
        * mm

    * String value representation - Ideally it should reference a formal specification but you are free to create you own vocabulary.
        * date-time `RFC 3339, section 5.6 <http://tools.ietf.org/html/rfc3339#section-5.6>`_
        * email `RFC 5322, section 3.4.1 <http://tools.ietf.org/html/rfc5322#section-3.4.1>`_
        * hostname `RFC 1034, section 3.1 <http://tools.ietf.org/html/rfc1034>`_
        * ipv4 `RFC 2673, section 3.2 <http://tools.ietf.org/html/rfc2673>`_
        * ipv6 `RFC 2373, section 2.2 <http://tools.ietf.org/html/rfc2373#section-2.2>`_
        * uri `RFC 3986 <http://tools.ietf.org/html/rfc3986>`_

    * How the representation is/can be used
        * data parsing and validation
        * unit conversion i.e., B to KiB
        * presentation i.e., graph labels, HTML links

* value_* (optional, value_type) - Array of values, only one type will be active at a time.

.. _stream_framing:

Stream Framing
==============

Heka has some custom framing that can be used to delimit records when
generating a stream of binary data. The entire structure encapsulating a
single message consists of a one byte record separator, one byte representing
the header length, a protobuf encoded message header, a one byte unit
separator, and the binary record content (usually a protobuf encoded Heka
message). This message structure is indicated in this diagram:

.. graphviz:: header.dot

The header schema is as follows:

* message_length (required, uint32) - length in bytes of the serialized message data
* hmac_hash_function (optional, int32) - enum indicating the hash function
  used to sign the message, 0 for MD5, 1, for SHA1
* hmac_signer (optional, string) - string token identifying HMAC signer
* hmac_key_version (optional, uint32) - version number of the provided HMAC key
* hmac (optional, []byte) - binary representation of provided HMAC key

Clients interested in decoding a Heka stream will need to read the header
length byte to determine the length of the header, extract the encoded header
data and decode this into a Header structure using an appropriate protobuf
library. From this they can then extract the length of the encoded message
data, which can then be extracted from the data stream and processed and/or
decoded as needed.
