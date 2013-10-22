define(
  [
    "underscore",
    "adapters/base_adapter",
    "models/plugin",
    "collections/plugins"
  ],
  function(_, BaseAdapter, Plugin, Plugins) {
    "use strict";

    /**
    * Adapter for getting plugins from the server. Consumes /data/heka_report.json.
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

    _.extend(PluginsAdapter.prototype, new BaseAdapter(), {
      /**
      * Fills globals, inputs, decoderPools, filters, and outputs with data fetched from the server.
      * Polls the server for updates after fetching data.
      *
      * @method fill
      */
      fill: function() {
        this.fetch("data/heka_report.json", function(response) {
          this.parseArrayIntoCollection(response.globals, this.globals);
          this.parseArrayIntoCollection(response.inputs, this.inputs);

          this.mapDecodersToPools(response.decoderPools, response.decoders);
          this.parseArrayIntoCollection(response.decoderPools, this.decoderPools);

          this.parseArrayIntoCollection(response.filters, this.filters);
          this.parseArrayIntoCollection(response.outputs, this.outputs);
        }.bind(this));

        this.pollForUpdates();
      },

      /**
      * Parses array returned from the server into a Plugins collection.
      *
      * @method parseArrayIntoCollection
      * @param {Object[]} array Array to be parsed.
      * @param {Plugins} collection Collection to be filled from parsed array.
      */
      parseArrayIntoCollection: function(array, collection) {
        var plugins = _.collect(array, function(p) {
          // No id is provided but the name is unique so use it as the id.
          var plugin = new Plugin(_.extend(p, { id: p.Name }));

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
      }
    });

    return PluginsAdapter;
  }
);
