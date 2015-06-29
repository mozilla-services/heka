define(
  [
    "backbone",
    "models/sandbox"
  ],
  function(Backbone, Sandbox) {
    "use strict";

    /**
    * Sandboxes collection.
    *
    * @class Sandboxes
    * @constructor
    */
    var Sandboxes = Backbone.Collection.extend({
      model: Sandbox,

      comparator: function(collection) {
        return(collection.get('Name'));
      }
    });

    return Sandboxes;
  }
);
