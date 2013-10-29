define(
  [
    "backbone",
    "deepModel"
  ],
  function(Backbone) {
    "use strict";

    /**
    * Specific output from a Sandbox.
    *
    * @class SandboxOutput
    *
    * @constructor
    */
    var SandboxOutput = Backbone.DeepModel.extend({});

    /**
    * Name of the sandbox output.
    *
    * @property {String} Name
    */

    /**
    * Filename of the sandbox output. These are relative paths that start with `data/*`.
    *
    * @property {String} Filename
    */

    /**
    * Data parsed from reading file found at Filename.
    *
    * @property {String,Object} data
    */

    return SandboxOutput;
  }
);
