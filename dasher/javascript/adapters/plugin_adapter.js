define(
  [
    "underscore",
    "backbone",
    "jquery",
    "models/plugin",
    "collections/plugins"
  ],
  function(_, Backbone, $, Plugin, Plugins) {
    "use strict";

    var PluginAdapter = function() {
      this.globals = new Plugins();
      this.inputs = new Plugins();
      this.decoderPools = new Plugins();
      this.filters = new Plugins();
      this.outputs = new Plugins();
    };

    _.extend(PluginAdapter.prototype, {
      fill: function() {
        this.fetch(function(response) {
          this.parseArrayIntoCollection(response.globals, this.globals);
          this.parseArrayIntoCollection(response.inputs, this.inputs);

          this.mapDecodersToPools(response.decoderPools, response.decoders);
          this.parseArrayIntoCollection(response.decoderPools, this.decoderPools);

          this.parseArrayIntoCollection(response.filters, this.filters);
          this.parseArrayIntoCollection(response.outputs, this.outputs);
        }.bind(this));

        this.listenForUpdates();
      },

      parseArrayIntoCollection: function(array, collection) {
        var plugins = _.collect(array, function(p) {
          // Use plugin name as its id
          var plugin = new Plugin(_.extend(p, { id: p.Name }));

          if (plugin.has("decoders")) {
            var decoders = new Plugins();

            this.parseArrayIntoCollection(plugin.get("decoders"), decoders);

            plugin.set("decoders", decoders);
          }

          return plugin;
        }.bind(this));

        if (collection.length > 0) {
          collection.set(plugins);
        } else {
          collection.reset(plugins);
        }
      },

      mapDecodersToPools: function(decoderPools, decoders) {
        _.each(decoderPools, function(decoderPool) {
          decoderPool.decoders = [];

          var basePoolName = decoderPool.Name.replace(/^DecoderPool\-/, "");

          _.each(decoders, function(decoder) {
            if (decoder.Name.indexOf(basePoolName) >= 0) {
              decoderPool.decoders.push(decoder);
            }
          });
        }.bind(this));
      },

      // Callback takes a response param.
      fetch: function(callback) {
        $.ajax("data/heka_report.json", { cache: false }).then(callback);
      },

      listenForUpdates: function() {
        setTimeout(function() { this.fill(); }.bind(this), 2000);
      }
    });

    return PluginAdapter;
  }
);
