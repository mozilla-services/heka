define(["views/base_view", "presenters/report_presenter", "hgn!templates/router_widget"], function(BaseView, ReportPresenter, RouterWidgetTemplate) {
  "use strict";

  var RouterWidget = BaseView.extend({
    presenter: ReportPresenter,
    template: RouterWidgetTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return RouterWidget;
});
