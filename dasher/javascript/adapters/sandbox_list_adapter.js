define(
  [
    "underscore",
    "backbone",
    "jquery",
    "models/sandbox"
  ],
  function(_, Backbone, $, Sandbox) {
    "use strict";

    var SandboxListAdapter = function() {
      this.sandboxes = new Backbone.Collection();
    };

    _.extend(SandboxListAdapter.prototype, {
      fill: function() {
        this.fetch(function(response) {
          this.parseArrayIntoCollection(response.sandboxes, this.sandboxes);
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
      fetch: function(callback) {
        $.ajax("data/sandboxes.json", { cache: false }).then(callback);
      }
    });

    return SandboxListAdapter;
  }
);
