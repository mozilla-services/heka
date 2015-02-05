/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Kun Liu (git@lk.vc)
#
# ***** END LICENSE BLOCK *****/

package smtp

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Each line of characters SHOULD be no more than 78 characters, excluding
// the CRLF. (RFC 2822 2.1.1)
const EMAIL_LINE_LENGTH = 78

func encodeBase64LimitChars(source string, limit int) (encoded string, numOfSourceChars int) {
	numOfSourceChars = limit / 4 * 3
	if len(source) <= numOfSourceChars {
		encoded = base64.StdEncoding.EncodeToString([]byte(source))
		numOfSourceChars = len(source)
	} else {
		for numOfSourceChars > 0 && !utf8.RuneStart(source[numOfSourceChars]) {
			numOfSourceChars--
		}
		if numOfSourceChars > 0 {
			encoded = base64.StdEncoding.EncodeToString([]byte(source[:numOfSourceChars]))
		}
	}
	return
}

func encodeQuoPriByte(dst []byte, b byte) int {
	dst[0] = '='
	dst[1] = fmt.Sprintf("%X", b>>4)[0]
	dst[2] = fmt.Sprintf("%X", b&0xf)[0]
	return 3
}

func encodeQuoPriLimitChars(source string, limit int) (encoded string, numOfSourceChars int) {
	buf := make([]byte, limit)
	var length int = 0
	for i := 0; i < len(source) && length < limit; {
		b := source[i]
		// 8-bit values which correspond to printable ASCII characters other
		// than "=", "?", and "_" (underscore), MAY be represented as those
		// characters. (RFC 2047 4.2.3)
		if b >= ' ' && b <= '~' && b != '=' && b != '?' && b != '_' {
			numOfSourceChars++
			if b == ' ' {
				buf[length] = '_' // RFC 2047 4.2.2
			} else {
				buf[length] = b
			}
			length++
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(source[i:])
		if r == utf8.RuneError {
			i++
			numOfSourceChars++
		} else if size <= 0 {
			break
		} else {
			if length+size*3 > limit {
				break
			}
			i += size
			numOfSourceChars += size
			for j := 0; j < size; j++ {
				length += encodeQuoPriByte(buf[length:], source[i+j])
			}
		}
	}
	return string(buf[:length]), numOfSourceChars
}

func isPrintableAscii(s string) bool {
	for _, c := range s {
		if c < ' ' || c > '~' {
			return false
		}
	}
	return true
}

func encodedField(field, s string) string {
	// only support 'Subject' field for now
	total := len(s)
	fieldNameLength := len(field) + 1 // ex. Subject:
	if total <= 0 || (total <= EMAIL_LINE_LENGTH-fieldNameLength-1 && isPrintableAscii(s)) {
		// no need encoding
		return field + ": " + s
	}
	// if needs encoding, resulting string will look like
	//   =?utf-8?B?5ZOI5ZOI77yM5oiR5piv5YiY5piG44CCMTIz?=\r\n
	//   =?utf-8?Q?_Hello!?=
	processed := 0
	lines := make([]string, 0, 1+len(s)/EMAIL_LINE_LENGTH)
	for processed < total {
		limit := EMAIL_LINE_LENGTH - fieldNameLength - len(" =?utf-8?B??=")
		fieldNameLength = 0 // only the first line has field name
		bStr, bNum := encodeBase64LimitChars(s[processed:], limit)
		qStr, qNum := encodeQuoPriLimitChars(s[processed:], limit)
		if bNum <= 0 && qNum <= 0 {
			// something wrong?
			return field + ": " + s
		}
		// use the most efficient encoding
		if qNum >= bNum {
			lines = append(lines, strings.Join([]string{" =", "utf-8", "Q", qStr, "="}, "?"))
			processed += qNum
		} else {
			lines = append(lines, strings.Join([]string{" =", "utf-8", "B", bStr, "="}, "?"))
			processed += bNum
		}
	}
	// each line of `lines` already has space as first character,
	// so use ":" instead of ": "
	return field + ":" + strings.Join(lines, "\r\n")
}

func encodeSubject(s string) string {
	return encodedField("Subject", s)
}
