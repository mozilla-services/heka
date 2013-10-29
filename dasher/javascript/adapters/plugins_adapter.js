define(
  [
    "underscore",
    "adapters/base_adapter",
    "adapters/sandboxes_adapter",
    "models/plugin",
    "collections/plugins"
  ],
  function(_, BaseAdapter, SandboxesAdapter, Plugin, Plugins) {
    "use strict";

    /**
    * Adapter for retrieving plugins from the server.
    *
    * Consumes `/data/heka_report.json`.
    *
    * @class PluginsAdapter
    * @extends BaseAdapter
    *
    * @constructor
    */
    var PluginsAdapter = function() {
      /**
      * Global plugins.
      *
      * @property {Plugins} globals
      */
      this.globals = new Plugins();

      /**
      * Input plugins.
      *
      * @property {Plugins} inputs
      */
      this.inputs = new Plugins();

      /**
      * Decoder pool plugins. These have decoders nested under each of them.
      *
      * @property {Plugins} decoderPools
      */
      this.decoderPools = new Plugins();

      /**
      * Decoder plugins.
      *
      * @property {Plugins} decoders
      */
      this.decoders = new Plugins();

      /**
      * Filter plugins.
      *
      * @property {Plugins} filters
      */
      this.filters = new Plugins();

      /**
      * Output plugins.
      *
      * @property {Plugins} outputs
      */
      this.outputs = new Plugins();
    };

    /**
    * Gets singleton instance of `PluginAdapter`
    *
    * @method instance
    *
    * @static
    */
    PluginsAdapter.instance = function() {
      if (!this._instance) {
        this._instance = new PluginsAdapter();
      }

      return this._instance;
    };

    _.extend(PluginsAdapter.prototype, new BaseAdapter(), {
      /**
      * Finds global plugin asynchronously.
      *
      * @method findGlobalWhere
      *
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findGlobalWhere: function(options, callback) {
        this.findWhere(this.globals, options, callback);
      },

      /**
      * Finds input plugin asynchronously.
      *
      * @method findGlobalWhere
      *
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findInputWhere: function(options, callback) {
        this.findWhere(this.inputs, options, callback);
      },

      /**
      * Finds decoder pool plugin asynchronously.
      *
      * @method findGlobalWhere
      *
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findDecoderPoolWhere: function(options, callback) {
        this.findWhere(this.decoderPools, options, callback);
      },

      /**
      * Finds decoder asynchronously.
      *
      * @method findGlobalWhere
      *
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findDecoderWhere: function(options, callback) {
        this.findWhere(this.decoders, options, callback);
      },

      /**
      * Finds filter plugin asynchronously.
      *
      * @method findGlobalWhere
      *
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findFilterWhere: function(options, callback) {
        this.findWhere(this.filters, options, callback);
      },


      /**
      * Finds output plugin asynchronously.
      *
      * @method findGlobalWhere
      *
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findOutputWhere: function(options, callback) {
        this.findWhere(this.outputs, options, callback);
      },

      /**
      * Fills globals, inputs, decoderPools, filters, and outputs with data fetched from the server.
      * Polls the server for updates after fetching data.
      *
      * @method fill
      *
      * @return {jQuery.Deferred} Deferred object that is resolved once the objects are filled
      */
      fill: function() {
        var deferred = $.Deferred();

        this.fetch("data/heka_report.json", function(response) {
          this.parseArrayIntoCollection(response.globals, this.globals, "Global");
          this.parseArrayIntoCollection(response.inputs, this.inputs, "Input");

          this.mapDecodersToPools(response.decoderPools, response.decoders);
          this.parseArrayIntoCollection(response.decoderPools, this.decoderPools, "DecoderPool");
          this.parseArrayIntoCollection(response.decoders, this.decoders, "Decoder");

          this.parseArrayIntoCollection(response.filters, this.filters, "Filter");
          this.mapSandboxOutputsToFilters(this.filters);

          this.parseArrayIntoCollection(response.outputs, this.outputs, "Output");

          deferred.resolve(this.globals, this.inputs, this.decoderPools, this.decoders, this.filters, this.outputs);

          this.pollForUpdates();
        }.bind(this));

        return deferred;
      },

      /**
      * Parses array returned from the server into a Plugins collection.
      *
      * @method parseArrayIntoCollection
      * @param {Object[]} array Array to be parsed.
      * @param {Plugins} collection Collection to be filled from parsed array.
      */
      parseArrayIntoCollection: function(array, collection, type) {
        var plugins = _.collect(array, function(p) {
          // No id is provided but the name is unique so use it as the id.
          var plugin = new Plugin(_.extend(p, { id: p.Name, Type: type }));

          // Convert decoders attribute into a plugins collection.
          if (plugin.has("decoders")) {
            var decoders = new Plugins();

            this.parseArrayIntoCollection(plugin.get("decoders"), decoders);

            plugin.set("decoders", decoders);
          }

          return plugin;
        }.bind(this));

        // If the collection already has data then we're doing an update so use set. Otherwise call
        // reset so that the views are properly rendered.
        if (collection.length > 0) {
          collection.set(plugins);
        } else {
          collection.reset(plugins);
        }
      },

      /**
      * Maps decoders array to corresponding decoderPools array.
      *
      * @method mapDecodersToPools
      * @param {Object[]} decoderPools Decoder pools array.
      * @param {Object[]} decoders Decoders array.
      */
      mapDecodersToPools: function(decoderPools, decoders) {
        _.each(decoderPools, function(decoderPool) {
          decoderPool.decoders = [];

          // Extract the name of the decoder from the pool name.
          var basePoolName = decoderPool.Name.replace(/^DecoderPool\-/, "");

          _.each(decoders, function(decoder) {
            if (decoder.Name.indexOf(basePoolName) >= 0) {
              decoderPool.decoders.push(decoder);
            }
          });
        }.bind(this));
      },

      /**
      * Maps sandboxes outputs onto the proper filters
      *
      * @method mapSandboxOutputsToFilters
      * @param {Plugins} filters Collection of filter plugins
      */
      mapSandboxOutputsToFilters: function(filters) {
        SandboxesAdapter.instance().fill().done(function(sandboxes) {
          sandboxes.forEach(function(sandbox) {
            var filter = filters.findWhere({ Name: sandbox.get("Name") });

            if (filter) {
              filter.set("Outputs", sandbox.get("Outputs"));
            }
          });
        });
      }
    });

    return PluginsAdapter;
  }
);
