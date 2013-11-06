define(
  [
    "backbone",
    "deepModel"
  ],
  function(Backbone) {
    "use strict";

    /**
    * Sandbox model representing data generated from sandbox plugins.
    *
    * @class Sandbox
    *
    * @constructor
    */
    var Sandbox = Backbone.DeepModel.extend({
      /**
      * Finds sandbox output by its short filename.
      *
      * @method findOutputByShortFilename
      *
      * @param {String} shortFilename Filename without `data/`.
      */
      findOutputByShortFilename: function(shortFilename) {
        var fileName = "data/" + shortFilename;
        var sandboxOutput;

        this.get("Outputs").forEach(function(o) {
          if (o.get("Filename") === fileName) {
            sandboxOutput = o;
          }
        });

        return sandboxOutput;
      }
    });

    /**
    * Unique identifier for the sandbox (same as `Name`).
    *
    * @property {String} id
    */

    /**
    * Name of the sandbox.
    *
    * @property {String} Name
    */

    /**
    * Outputs in the sandbox.
    *
    * @property {Backbone.Collection} outputs
    */

    return Sandbox;
  }
);
