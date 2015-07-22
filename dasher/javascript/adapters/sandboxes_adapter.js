define(
  [
    "jquery",
    "underscore",
    "backbone",
    "adapters/base_adapter",
    "models/sandbox",
    "models/sandbox_output"
  ],
  function($, _, Backbone, BaseAdapter, Sandbox, SandboxOutput) {
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
    var Sandboxes = Backbone.Collection.extend({
      model: Sandbox,

      comparator: function(collection) {
        return(collection.get('Name'));
      }
    });

    var SandboxesAdapter = function() {
      /**
      * Sandboxes collection to be filled with data.
      *
      * @property {Backbone.Collection} sandboxes
      */
      this.sandboxes = new Sandboxes();
    };

    /**
    * Gets singleton instance of `SandboxesAdapter`
    *
    * @method instance
    *
    * @static
    */
    SandboxesAdapter.instance = function() {
      if (!this._instance) {
        this._instance = new SandboxesAdapter();
      }

      return this._instance;
    };

    _.extend(SandboxesAdapter.prototype, new BaseAdapter(), {
      /**
      * Finds sandbox asynchronously.
      *
      * @method findSandboxWhere
      *
      * @param {Object} options Search options that are passed options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findSandboxWhere: function(options, callback) {
        this.findWhere(this.sandboxes, options, callback);
      },

      /**
      * Fills sandboxes with data fetched from the server. Polls the server for updates after
      * fetching data.
      *
      * @method fill
      */
      fill: function(enablePolling) {
        var deferred = $.Deferred();

        this.fetch("data/sandboxes.json", function(response) {
          this.parseArrayIntoCollection(response.sandboxes, this.sandboxes);

          deferred.resolve(this.sandboxes);

          if (enablePolling) {
            this.pollForUpdates();
          }
        }.bind(this));

        return deferred;
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
          // Add sandbox name to output for convenience
          _.each(s.Outputs, function(o) {
            o.SandboxName = s.Name;
          });

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
