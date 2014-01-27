define(
  [
    "underscore",
    "jquery",
    "adapters/base_adapter",
    "lib/circular_buffer"
  ],
  function(_, $, BaseAdapter, CircularBuffer) {
    "use strict";

    /**
    * Adapter for retrieving circular buffer data from sandbox outputs.
    *
    * Consumes `/data/*.cbuf`.
    *
    * @class SandboxOutputCbufAdapter
    * @extends BaseAdapter
    *
    * @constructor
    *
    * @param {SandboxOutput} sandboxOutput SandboxOutput to be filled by the adapter
    */
    var SandboxOutputCbufAdapter = function(sandboxOutput) {
      /**
      * SandboxOutput to be filled by the adapter.
      *
      * @property {SandboxOutput} sandboxOutput
      */
      this.sandboxOutput = sandboxOutput;
    };

    _.extend(SandboxOutputCbufAdapter.prototype, new BaseAdapter(), {
      /**
      * Fills sandboxOutput with data fetched from the server. Sets annotations, header, and data
      * attributes on sandboxOutput. Polls the server for updates after fetching data.
      *
      * @method fill
      */
      fill: function() {
        this.fetch(this.sandboxOutput.get("Filename"), function(response) {
          var circularBuffer = CircularBuffer.parse(response);

          this.sandboxOutput.set({
            options: circularBuffer.options,
            annotations: circularBuffer.annotations,
            header: circularBuffer.header,
            data: circularBuffer.data
          });
        }.bind(this));

        this.pollForUpdates();
      }
    });

    return SandboxOutputCbufAdapter;
  }
);
