define(
  [
    "underscore",
    "backbone",
    "jquery",
    "adapters/base_adapter",
    "lib/circular_buffer_parser"
  ],
  function(_, Backbone, $, BaseAdapter, CircularBufferParser) {
    "use strict";

    var SandboxOutputCbufAdapter = function(sandboxOutput) {
      this.sandboxOutput = sandboxOutput;
    };

    _.extend(SandboxOutputCbufAdapter.prototype, new BaseAdapter(), {
      fill: function() {
        this.fetch(this.sandboxOutput.get("Filename"), function(response) {
          var circularBuffer = CircularBufferParser.parse(response);

          this.sandboxOutput.set({
            annotations: circularBuffer.annotations,
            header: circularBuffer.header,
            data: circularBuffer.data
          });
        }.bind(this));

        this.listenForUpdates();
      }
    });

    return SandboxOutputCbufAdapter;
  }
);
