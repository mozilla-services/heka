/*

Pipeline is the main unit of operation for hekad, it contains the
implementation and specification for Plugins, Inputs, Decoders,
Filters and Outputs.

Inputs, Decoders, Filters, and Outputs all implement the Plugin interface
as well as adding their own API necessary to be valid for that type of
Plugin.

*/
package pipeline
