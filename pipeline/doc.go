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
#
# ***** END LICENSE BLOCK *****/

/*

Implementation and specification for Plugins, Inputs, Decoders,
Filters, Outputs, and the routing.

Inputs, Decoders, Filters, and Outputs all implement the Plugin
interface as well as adding their own API necessary to be valid for
that type of Plugin.

*/
package pipeline
