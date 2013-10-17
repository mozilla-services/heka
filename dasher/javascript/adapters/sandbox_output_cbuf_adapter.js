define(
  [
    "underscore",
    "backbone",
    "jquery",
    "lib/circular_buffer_parser"
  ],
  function(_, Backbone, $, CircularBufferParser) {
    "use strict";

    var SandboxOutputCbufAdapter = function(sandboxOutput) {
      this.sandboxOutput = sandboxOutput;
    };

    _.extend(SandboxOutputCbufAdapter.prototype, {
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
      },

      // Callback takes a response param.
      fetch: function(endpoint, callback) {
        $.ajax(endpoint, { cache: false }).then(callback);
      },

      listenForUpdates: function() {
        setTimeout(function() { this.fill(); }.bind(this), 1000);
      }
    });

    return SandboxOutputCbufAdapter;
  }
);
