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
    var Sandbox = Backbone.DeepModel.extend({});

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
