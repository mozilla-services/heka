define(
  [
    "underscore",
    "jquery",
    "backbone",
    "adapters/base_adapter",
    "lib/termination_report"
  ],
  function(_, $, Backbone, BaseAdapter, TerminationReport) {
    "use strict";

    /**
    * Adapter for retrieving the sandbox termination report.
    *
    * Consumes `/data/heka_sandbox_termination.tsv`.
    *
    * @class TerminationReportAdapter
    * @extends BaseAdapter
    *
    * @constructor
    */
    var TerminationReportAdapter = function() {
      /**
      * Termination report collection to be filled by the adapter.
      *
      * @property {Backbone.Collection} terminationReportCollection
      */
      this.terminationReportCollection = new Backbone.Collection();
    };

    _.extend(TerminationReportAdapter.prototype, new BaseAdapter(), {
      /**
      * Fills terminationReportCollection with data fetched from the server.
      *
      * @method fill
      */
      fill: function() {
        this.fetch("data/heka_sandbox_termination.tsv", function(response) {
          var terminationReport = TerminationReport.parse(response);

          this.terminationReportCollection.set(terminationReport.data);
        }.bind(this));

        this.pollForUpdates(5000);
      }
    });

    return TerminationReportAdapter;
  }
);
