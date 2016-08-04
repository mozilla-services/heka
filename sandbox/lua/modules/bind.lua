-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[BIND DNS server log grammars and functions

This module contains functions and grammars for processing logs from
the ISC Berkeley Internet Name Daemon DNS server:

https://www.isc.org/downloads/bind/

Adapted from: https://github.com/mozilla-services/heka/wiki/How-to-convert-a-PayloadRegex-MultiDecoder-to-a-SandboxDecoder-using-an-LPeg-Grammar

Built with the help of: http://lpeg.trink.com/

Reference for LPEG functions and uses: http://www.inf.puc-rio.br/~roberto/lpeg/lpeg.html

Sources for explanations on what the DNS query flags mean:

https://deepthought.isc.org/article/AA-00434/0/What-do-EDC-and-other-letters-I-see-in-my-query-log-mean.html

http://jpmens.net/2011/02/22/bind-querylog-know-your-flags/


Sample BIND query log message, with the print-category, print-severity and print-time options
all set to 'yes' in the logging channel options in named.conf:

27-May-2015 21:06:49.246 queries: info: client 10.0.1.70#41242 (webserver.company.com): query: webserver.company.com IN A +E (10.0.1.71)

The things we want out of it are:

* The client IP
* The name that was queried
* The domain of the name that was queried
* The record type (A, MX, PTR, etc.)
* The address of the interface that BIND used for the reply

--]]

local l = require "lpeg"
local math = require "math"
local string = require "string"
local date_time = require "date_time"
local ip = require "ip_address"
l.locale(l)

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module

--local formats  = read_config("formats")
--Read the "type" value from the config:
--local msg_type = read_config("type")

--[[ Generic patterns --]]
--Patterns for strings in the log lines that don't change from query to query:
local space = l.space
-- ':'
local colon_literal = l.P":"
-- 'queries'
local queries_literal = l.P"queries:"
-- '#'
local pound_literal = l.P"#" 
-- 'info'
local info_literal = l.P"info:"
-- 'client'
local client_literal = l.P"client"
-- '('
local open_paren_literal = l.P"("
-- ')'
local close_paren_literal = l.P")"
-- 'query'
local query_literal = l.P"query:"
-- 'IN' literal string; 
local in_literal = l.P"IN"
-- '+' literal character; + indicates that recursion was requested.
local plus_literal = l.P"+"
-- '-' literal character; - indicates that recursion was not requested.
local minus_literal = l.P"-"
-- 'E' literal character; E indicates that extended DNS was used
-- Source: http://ftp.isc.org/isc/bind9/cur/9.9/doc/arm/Bv9ARM.ch06.html#id2575001
-- More about EDNS: https://en.wikipedia.org/wiki/Extension_mechanisms_for_DNS
local e_literal = l.P"E"
-- 'S' literal character; s indicates that the query was signed
-- Source: http://ftp.isc.org/isc/bind9/cur/9.9/doc/arm/Bv9ARM.ch06.html#id2575001
local s_literal = l.P"S"
--'D' literal character; D means the client wants any DNSSEC related data
-- Source: http://ftp.isc.org/isc/bind9/cur/9.9/doc/arm/Bv9ARM.ch06.html#id2575001
local d_literal = l.P"D"
--'T' literal character; if TCP was used
-- Source: http://ftp.isc.org/isc/bind9/cur/9.9/doc/arm/Bv9ARM.ch06.html#id2575001
local t_literal = l.P"T"
--'C' literal character; queryer wants an answer anyway even if DNSSEC validation checks fail
-- Source: http://ftp.isc.org/isc/bind9/cur/9.9/doc/arm/Bv9ARM.ch06.html#id2575001
local c_literal = l.P"C"


--[[ More complicated patterns for things that do change from line to line: --]]

--The below pattern matches date/timestamps in the following format:
-- 27-May-2015 21:06:49.246
-- The milliseconds (the .246) are discarded by the `l.P"." * l.P(3)` at the end:
--Source: https://github.com/mozilla-services/lua_sandbox/blob/dev/modules/date_time.lua
local timestamp = l.Cg(date_time.build_strftime_grammar("%d-%b-%Y %H:%M:%S") / date_time.time_to_ns, "Timestamp") * l.P"." * l.P(3)
local x4            = l.xdigit * l.xdigit * l.xdigit * l.xdigit

--The patterns below match IPv4 addresses from BIND query logs like the following:
-- 10.0.1.70#41242

--Ephemeral ports can be up to 5 digits long (^-5 means match up to and including 5 instances of the pattern):
local client_ephemeral_port = l.digit^-5
local client_ip = l.Cg(l.Ct(l.Cg(ip.v4, "value") * l.Cg(l.Cc"ipv4", "representation")), "ClientIP") * pound_literal * l.Cg(client_ephemeral_port, "ClientEphemeralPort")

--The ends of query logs have the IP address the DNS server used to respond to the query with. The LPEG capture group is just like 
-- the client IP, but encased in ( ) and without the # literal at the front: (10.0.1.71)
local server_responding_ip = l.P"(" * l.Cg(l.Ct(l.Cg(ip.v4, "value") * l.Cg(l.Cc"ipv4", "representation")), "ServerRespondingIP") * l.P")"

--[[DNS query record types:
Create a capture group that will match the DNS record type.

The + signs mean to select A or CNAME or MX or PTR and so on.

The ', "record_type"' part sets the name of the capture's entry in the table of
matches that gets built.

Source: https://en.wikipedia.org/wiki/List_of_DNS_record_types


The patterns for A and AAAA records are at the bottom because the whole listing of 
possible record types inside of the l.Cg() group is order-sensitive. 

Putting the pattern for "A" at the top would mean that "A" would match, but "AXFR",
for instance, would not. The "A" in "AXFR" match, but LPEG would barf on the "XFR",
because the grammar for the whole log line expects a space after whatever it successfully
matches into the `dns_record_type` variable.

"AAAA" is at the bottom, but above "A" for the same reason: a match for "A" would only hit
the first "A" inside of "AAAA", then the grammar would break because it's expecting a space
after the match for `dns_record_type` instead of "XFR"
--]]
dns_record_type = l.Cg(
      l.P"AFSDB" /"AFSDB"
    + l.P"APL" /"APL"
    + l.P"AXFR" /"AXFR"
    + l.P"CAA" /"CAA"
    + l.P"CDNSKEY" /"CDNSKEY"
    + l.P"CDS" /"CDS"
    + l.P"CERT" /"CERT"
    + l.P"CNAME" /"CNAME"
    + l.P"DHCID" /"DHCID"
    + l.P"DLV" /"DLC"
    + l.P"DNAME" /"DNAME"
    + l.P"DS" /"DS"
    + l.P"HIP" /"HIP"
    + l.P"IPSECKEY" /"IPSECKEY"
    + l.P"IXFR" /"IXFR"
    + l.P"KEY" /"KEY"
    + l.P"KX" /"KX"
    + l.P"LOC" /"LOC"
    + l.P"MX" /"MX"
    + l.P"NAPTR" /"NAPTR"
    + l.P"NSEC3PARAM" /"NSEC3PARAM"
    + l.P"NSEC3" /"NSEC3"
    + l.P"NSEC" /"NSEC"
    + l.P"NS" /"NS"
    + l.P"OPT" /"OPT"
    + l.P"PTR" /"PTR"
    + l.P"RRSIG" /"RRSIG"
    + l.P"RP" /"RP"
    + l.P"SIG" /"SIG"
    + l.P"SOA" /"SOA"
    + l.P"SRV" /"SRV"
    + l.P"SSHFP" /"SSHFP"
    + l.P"TA" /"TA"
    + l.P"TKEY" /"TKEY"
    + l.P"TLSA" /"TLSA"
    + l.P"TSIG" /"TSIG"
    + l.P"TXT" /"TXT"
    + l.P"*" /"*"
    + l.P"A"^4 /"AAAA"
    + l.P"A"^-1 /"A"
    , "RecordType")

--A capture group for the 3 kinds of DNS record classes.
-- Source: https://en.wikipedia.org/wiki/Domain_Name_System#DNS_resource_records
dns_record_class = l.Cg(
      --For internet records:
      l.P"IN" /"IN"
      --For CHAOS records:
    + l.P"CH" /"CH"
      --For Hesiod records:
    + l.P"HS" /"HS"
    , "RecordClass")

--[[Query flag patterns
Sources on what the query flags mean: 

* https://deepthought.isc.org/article/AA-00434/0/What-do-EDC-and-other-letters-I-see-in-my-query-log-mean.html
* http://jpmens.net/2011/02/22/bind-querylog-know-your-flags/

]]--

--Create a table to hold possible matches that may appear:
local t = {
      ["+"] = "recursion requested",
      ["-"] = "recursion not requested",
      E = "EDNS used",
      S = "query signed",
      D = "DNSSEC data wanted",
      C = "no DNSSEC validation check",
      T = "TCP used",
}

--Create a capture group that uses the table above to match any one or more of '+-EDCST'
--that are present and add them to a new table called QueryFlags: 
query_flags = l.Cg( l.Ct((l.S"+-EDCST" / t)^1), "QueryFlags" )

--[[Hostname and domain name patterns

Hostnames and domain names are broken up into fragments that are called 
"labels", which are the parts between the dots.

Source: https://en.wikipedia.org/wiki/Hostname#Restrictions_on_valid_host_names

For instance, webserver.company.com has the labels "webserver", "company" and 
"com"

The pattern below uses the upper, lower and digit shortcuts from the main LPEG 
library and combines them with the hyphen (-) character to match anything that's
part of a valid hostname label. The ^1 means match one or more instances of it.
--]]
local hostname_fragment = (l.upper + l.lower + l.digit +  "-" + "_")^1

--The pattern below matches one or more hostname_fragments, followed by a . 
--and followed by one more hostname_fragment, indicating the end of a complete
--hostname. The open and close parens and colon are to match the decorations
--BIND puts around the name: 
-- (webserver.company.com):
local enclosed_query = "(" * l.Cg((hostname_fragment * ".")^1 * hostname_fragment, "FullQuery") * "):"

--The ^-1 means match at most 1 instance of the pattern. We want this so that we 
--can match the first part of a hostname and leave the rest for the l.Cg((hostname_fragment...
--capture group to match into the QueryDomain table entry.
--In webserver.company.com, `(hostname_fragment * ".")^-1` matches webserver.
--and `l.Cg((hostname_fragment...` matches company.com
--Match 0 or more (what the ^0 does) hostname_fragments and trailing '.' characters after a first hostname fragment.
--We're matching 0 or more because this will include queries for 2-fragment queries, like 'google.com'
--If we did ^1 instead of ^0, server.google.com would successfully get parsted but goole.com wouldn't since 'google' would get captured
--as the QueryName and there'd be no '.' left for the QueryDomain capture group to capture if it *had* to capture 1 or more because of a ^1.
--Also match 0 or more hostname_fragments and trailing "." characters for the QueryDomain since some DNS queries may not have a domain, like a query for 'github'
--instead of github.com:
local query = l.Cg((hostname_fragment)^0, "QueryName") * l.P"."^0 * l.Cg((hostname_fragment * ".")^0 * hostname_fragment, "QueryDomain")^0

--Use all of the previously defined patterns to build a grammar:
query_log_grammar = timestamp * space * queries_literal * space * info_literal * space * client_literal * space * client_ip * space * enclosed_query * space * query_literal * space * query * space * dns_record_class * space * dns_record_type * space * query_flags * space * server_responding_ip

-- To use the above grammar on http://lpeg.trink.com/, paste in everything above this line and just the single line below (uncomment it too):
--grammar = l.Ct(bind_query)

return M