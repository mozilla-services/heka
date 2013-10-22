define(
  [
    "underscore",
    "adapters/base_adapter"
  ],
  function(_, BaseAdapter) {
    "use strict";

    /**
    * Adapter for retrieving plain text data from sandbox outputs.
    *
    * Consumes `/data/*.txt`.
    *
    * Note: This is the fallback sandbox output adapter so it can fetch any type of document that
    * isn't a circular buffer.
    *
    * @class SandboxOutputTxtAdapter
    * @extends BaseAdapter
    *
    * @constructor
    *
    * @param {SandboxOutput} sandboxOutput SandboxOutput to be filled by the adapter
    */
    var SandboxOutputTxtAdapter = function(sandboxOutput) {
      /**
      * SandboxOutput to be filled by the adapter.
      *
      * @property {SandboxOutput} sandboxOutput
      */
      this.sandboxOutput = sandboxOutput;
    };

    _.extend(SandboxOutputTxtAdapter.prototype, new BaseAdapter(), {
      /**
      * Fills sandboxOutput with data fetched from the server. Sets the data attribute on
      * sandboxOutput. Polls the server for updates after fetching data.
      *
      * @method fill
      */
      fill: function() {
        this.fetch(this.sandboxOutput.get("Filename"), function(response) {
          this.sandboxOutput.set("data", response);

          this.pollForUpdates();
        }.bind(this));
      }
    });

    return SandboxOutputTxtAdapter;
  }
);
