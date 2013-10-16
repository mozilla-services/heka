define(
  [
    "underscore",
    "backbone",
    "jquery",
    "models/sandbox",
    "lib/circular_buffer_parser"
  ],
  function(_, Backbone, $, Sandbox, CircularBufferParser) {
    "use strict";

    var SandboxSourceCbufAdapter = function(sandbox) {
      this.sandbox = sandbox;
    };

    _.extend(SandboxSourceCbufAdapter.prototype, {
      fill: function() {
        this.fetch(this.sandbox.get("Filename"), function(response) {
          var circularBuffer = CircularBufferParser.parse(response);

          this.sandbox.set({
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

    return SandboxSourceCbufAdapter;
  }
);
