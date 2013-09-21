define(["views/base_view", "presenters/report_presenter", "hgn!templates/reports_row"], function(BaseView, ReportPresenter, ReportsRowTemplate) {
  "use strict";

  var ReportsRow = BaseView.extend({
    tagName: "tr",
    presenter: ReportPresenter,
    template: ReportsRowTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return ReportsRow;
});
