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
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

/*

The `pipeline` package implements the `hekad` daemon's main message processing
pipeline. It defines interface specifications for plugins in general, and for
all of the various specific plugin types (i.e. Input, Decoder, Filter, and
Output), as well as implementations of a number of plugins of each type. It
also defines and provides a variety of interfaces through which plugins can
interact with the Heka environment.

For more information about Heka please see the following resources:

	* Heka project documentation: http://heka-docs.readthedocs.org/
	* `hekad` daemon documenation: http://hekad.readthedocs.org/
	* `hekad` github project: https://github.com/mozilla-services/heka

*/
package pipeline
