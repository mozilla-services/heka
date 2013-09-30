define(
  [
    "backbone",
    "models/plugin"
  ],
  function(Backbone, Plugin) {
    "use strict";

    var Plugins = Backbone.Collection.extend({
      model: Plugin,

      getRouter: function() {
        return this.findWhere({ id: "Router" });
      },

      getInputRecycleChan: function() {
        return this.findWhere({ id: "inputRecycleChan" });
      },

      getInjectRecycleChan: function() {
        return this.findWhere({ id: "injectRecycleChan" });
      }
    });

    return Plugins;
  }
);
