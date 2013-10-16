define(
  [
    "underscore",
    "backbone",
    "jquery"
  ],
  function(_, Backbone, $) {
    "use strict";

    var SandboxSourceTxtAdapter = function(sandboxSource) {
      this.sandboxSource = sandboxSource;
    };

    _.extend(SandboxSourceTxtAdapter.prototype, {
      fill: function() {
        this.fetch(this.sandboxSource.get("Filename"), function(response) {
          this.sandboxSource.set("data", response);

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

    return SandboxSourceTxtAdapter;
  }
);
