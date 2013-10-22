define(
  [
    "underscore"
  ],
  function(_) {
    "use strict";

    /**
    * Presents a SandboxOutput for use in a view.
    *
    * @class SandboxOutputPresenter
    * @extends SandboxOutput
    *
    * @constructor
    *
    * @param {SandboxOutput} sandboxOutput SandboxOutput to be presented.
    */
    var SandboxOutputPresenter = function (sandboxOutput) {
      _.extend(this, sandboxOutput.attributes);
    };

    _.extend(SandboxOutputPresenter.prototype, {
      /**
      * Filename without the leading `data/`.
      *
      * @method ShortFilename
      * @return {String} shortened Filename
      */
      ShortFilename: function() {
        return this.Filename.replace(/^data\//, "");
      }
    });

    return SandboxOutputPresenter;
  }
);
