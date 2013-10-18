define(
  [
    "underscore",
    "backbone",
    "jquery",
    "adapters/base_adapter",
    "models/sandbox"
  ],
  function(_, Backbone, $, BaseAdapter, Sandbox) {
    "use strict";

    var SandboxesAdapter = function() {
      this.sandboxes = new Backbone.Collection();
    };

    _.extend(SandboxesAdapter.prototype, new BaseAdapter(), {
      fill: function() {
        this.fetch("data/sandboxes.json", function(response) {
          this.parseArrayIntoCollection(response.sandboxes, this.sandboxes);

          this.listenForUpdates();
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
      }
    });

    return SandboxesAdapter;
  }
);
