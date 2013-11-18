define(
  [
    "views/base_view",
    "hgn!templates/termination_report/termination_report_index",
    "adapters/termination_report_adapter",
    "presenters/termination_report_row_presenter"
  ],
  function(BaseView, TerminationReportIndexTemplate, TerminationReportAdapter, TerminationReportRowPresenter) {
    "use strict";

    /**
    * Index view for sandboxes. This is a top level view that's loaded by the router.
    *
    * @class TerminationReportIndex
    * @extends BaseView
    *
    * @constructor
    */
    var TerminationReportIndex = BaseView.extend({
      presenter: TerminationReportRowPresenter,
      template: TerminationReportIndexTemplate,

      initialize: function() {
        this.adapter = new TerminationReportAdapter();
        this.collection = this.adapter.terminationReportCollection;

        this.listenTo(this.collection, "add remove reset change", this.render, this);

        this.adapter.fill();
      },

      /**
      * Stops polling for updates before being destroyed.
      *
      * @method beforeDestroy
      */
      beforeDestroy: function() {
        this.adapter.stopPollingForUpdates();
      }
    });

    return TerminationReportIndex;
  }
);
