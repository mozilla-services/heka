define(
  [
    "underscore",
    "backbone",
    "jquery"
  ],
  function(_, Backbone, $) {
    "use strict";

    var SandboxOutputTxtAdapter = function(sandboxOutput) {
      this.sandboxOutput = sandboxOutput;
    };

    _.extend(SandboxOutputTxtAdapter.prototype, {
      fill: function() {
        this.fetch(this.sandboxOutput.get("Filename"), function(response) {
          this.sandboxOutput.set("data", response);

          this.listenForUpdates();
        }.bind(this));
      },

      // Callback takes a response param.
      fetch: function(endpoint, callback) {
        $.ajax(endpoint, { cache: false }).then(callback);
      },

      listenForUpdates: function() {
        setTimeout(function() { this.fill(); }.bind(this), 1000);
      }
    });

    return SandboxOutputTxtAdapter;
  }
);
