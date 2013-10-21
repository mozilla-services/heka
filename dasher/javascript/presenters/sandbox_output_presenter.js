define(
  [
    "underscore"
  ],
  function(_) {
    "use strict";

    var SandboxOutputPresenter = function (sandboxOutput) {
      _.extend(this, sandboxOutput.attributes);
    };

    _.extend(SandboxOutputPresenter.prototype, {
      ShortFilename: function() {
        return this.Filename.replace(/^data\//, "");
      }
    });

    return SandboxOutputPresenter;
  }
);
