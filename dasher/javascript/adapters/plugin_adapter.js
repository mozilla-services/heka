define(["underscore", "backbone", "jquery", "models/plugin", "collections/plugins"], function(_, Backbone, $, Plugin, Plugins) {
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
      this.fetch(_.bind(function(response) {
        this.parseArrayIntoCollection(this.globals, response.globals);
        this.parseArrayIntoCollection(this.inputs, response.inputs);
        this.parseArrayIntoCollection(this.decoderPools, response.decoderPools);
        this.parseArrayIntoCollection(this.filters, response.filters);
        this.parseArrayIntoCollection(this.outputs, response.outputs);
      }, this));

      // TODO: Connect to event source for updates
    },

    parseArrayIntoCollection: function(collection, array) {
      var plugins = _.collect(array, function(plugin) {
        // Use plugin name as its id
        return new Plugin(_.extend(plugin, { id: plugin.Plugin }));
      });

      if (collection.length > 0) {
        collection.set(plugins);
      } else {
        collection.reset(plugins);
      }
    },

    // Callback takes a response param.
    fetch: function(callback) {
      $.getJSON("sample_data/heka_report_new.json").then(callback);
    }
  });

  return PluginAdapter;
});
