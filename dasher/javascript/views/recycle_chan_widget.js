define(["views/base_view", "presenters/report_presenter", "hgn!templates/recycle_chan_widget"], function(BaseView, ReportPresenter, RecycleChanWidgetTemplate) {
  "use strict";

  var RecycleChanWidget = BaseView.extend({
    presenter: ReportPresenter,
    template: RecycleChanWidgetTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return RecycleChanWidget;
});
