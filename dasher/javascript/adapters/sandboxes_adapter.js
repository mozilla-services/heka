define(
  [
    "underscore",
    "backbone",
    "jquery",
    "models/sandbox"
  ],
  function(_, Backbone, $, Sandbox) {
    "use strict";

    var SandboxesAdapter = function() {
      this.sandboxes = new Backbone.Collection();
    };

    _.extend(SandboxesAdapter.prototype, {
      fill: function() {
        this.fetch(function(response) {
          this.parseArrayIntoCollection(response.sandboxes, this.sandboxes);
        }.bind(this));
      },

      parseArrayIntoCollection: function(array, collection) {
        var sandboxes = _.collect(array, function(s) {
          return new Sandbox(_.extend(s, { id: s.Name, Outputs: new Backbone.Collection(s.Outputs) }));
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

    return SandboxesAdapter;
  }
);
