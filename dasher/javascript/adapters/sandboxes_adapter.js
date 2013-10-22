define(
  [
    "underscore",
    "backbone",
    "adapters/base_adapter",
    "models/sandbox",
    "models/sandbox_output"
  ],
  function(_, Backbone, BaseAdapter, Sandbox, SandboxOutput) {
    "use strict";

    /**
    * Adapter for retrieving sandboxes from the server.
    *
    * Consumes `/data/sandboxes.json`.
    *
    * @class SandboxesAdapter
    * @extends BaseAdapter
    *
    * @constructor
    */
    var SandboxesAdapter = function() {
      /**
      * Sandboxes collection to be filled with data.
      *
      * @property {Backbone.Collection} sandboxes
      */
      this.sandboxes = new Backbone.Collection();
    };

    _.extend(SandboxesAdapter.prototype, new BaseAdapter(), {
      /**
      * Fills sandboxes with data fetched from the server. Polls the server for updates after
      * fetching data.
      *
      * @method fill
      */
      fill: function() {
        this.fetch("data/sandboxes.json", function(response) {
          this.parseArrayIntoCollection(response.sandboxes, this.sandboxes);

          this.pollForUpdates();
        }.bind(this));
      },

      /**
      * Parses array returned from the server into a collection.
      *
      * @method parseArrayIntoCollection
      * @param {Object[]} array Array to be parsed
      * @param {Backbone.Collection} collection Collection to be filled from parsed array
      */
      parseArrayIntoCollection: function(array, collection) {
        var sandboxes = _.collect(array, function(s) {
          var outputs = new Backbone.Collection(s.Outputs, { model: SandboxOutput });
          // No id is provided but the name is unique so use it as the id
          return new Sandbox(_.extend(s, {id: s.Name, Outputs: outputs }));
        });

        // If the collection already has data then we're doing an update so use set. Otherwise call
        // reset so that the views are properly rendered.
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
