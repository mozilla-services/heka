define(
  [
    "backbone",
    "models/plugin"
  ],
  function(Backbone, Plugin) {
    "use strict";

    /**
    * Plugins collection.
    *
    * @class Plugins
    * @constructor
    */
    var Plugins = Backbone.Collection.extend({
      model: Plugin,

      /**
      * Gets the Router plugin from the collection.
      *
      * @method getRouter
      *
      * @return {Plugin} Router plugin
      */
      getRouter: function() {
        return this.findWhere({ id: "Router" });
      },

      /**
      * Gets the InputRecycleChan plugin from the collection.
      *
      * @method getInputRecycleChan
      *
      * @return {Plugin} InputRecycleChan plugin
      */
      getInputRecycleChan: function() {
        return this.findWhere({ id: "inputRecycleChan" });
      },

      /**
      * Gets the InjectRecycleChan plugin from the collection.
      *
      * @method getInjectRecycleChan
      *
      * @return {Plugin} InjectRecycleChan plugin
      */
      getInjectRecycleChan: function() {
        return this.findWhere({ id: "injectRecycleChan" });
      },

      comparator: function(collection) {
        return(collection.get('Name'));
      }
    });

    return Plugins;
  }
);
