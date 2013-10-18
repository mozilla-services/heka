define(
  [
    "underscore",
    "backbone",
    "jquery",
    "adapters/base_adapter"
  ],
  function(_, Backbone, $, BaseAdapter) {
    "use strict";

    var SandboxOutputTxtAdapter = function(sandboxOutput) {
      this.sandboxOutput = sandboxOutput;
    };

    _.extend(SandboxOutputTxtAdapter.prototype, new BaseAdapter(), {
      fill: function() {
        this.fetch(this.sandboxOutput.get("Filename"), function(response) {
          this.sandboxOutput.set("data", response);

          this.listenForUpdates();
        }.bind(this));
      }
    });

    return SandboxOutputTxtAdapter;
  }
);
