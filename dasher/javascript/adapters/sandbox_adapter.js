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

    var SandboxAdapter = function(sandbox) {
      this.sandbox = sandbox;
    };

    _.extend(SandboxAdapter.prototype, {
      fill: function() {
        this.fetchHistoricalData(this.sandbox.get("historicalEndpoint"), function(response) {
          var circularBuffer = CircularBufferParser.parse(response);

          this.sandbox.set({
            annotations: circularBuffer.annotations,
            header: circularBuffer.header,
            data: circularBuffer.data
          });
        }.bind(this));
      },

      parseArrayIntoCollection: function(array, collection) {
        var sandboxes = _.collect(array, function(s) {
          return new Sandbox(s);
        });

        if (collection.length > 0) {
          collection.set(sandboxes);
        } else {
          collection.reset(sandboxes);
        }
      },

      // Callback takes a response param.
      fetchHistoricalData: function(endpoint, callback) {
        $.ajax(endpoint, { cache: false }).then(callback);
      }
    });

    return SandboxAdapter;
  }
);
