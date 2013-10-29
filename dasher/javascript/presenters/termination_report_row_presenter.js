define(
  [
    "underscore",
    "moment"
  ],
  function(_, moment) {
    "use strict";

    /**
    * Presents a `TerminationReport` row for use in a view.
    *
    * @class TerminationReportRowPresenter
    *
    * @constructor
    *
    * @param {Plugin} plugin Plugin to be presented
    */
    var TerminationReportRowPresenter = function (termination_report_row) {
      _.extend(this, termination_report_row.attributes);
    };

    _.extend(TerminationReportRowPresenter.prototype, {
      /**
      * Format the date and time.
      *
      * @method formattedTime
      * @return {String} Formatted time e.g. Oct 28 2013 2:48 PM
      */
      formattedTime: function() {
        return moment(this.time).format("lll");
      }
    });

    return TerminationReportRowPresenter;
  }
);
