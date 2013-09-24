define(["views/base_view", "presenters/plugin_presenter", "hgn!templates/health/recycle_chan_widget"], function(BaseView, PluginPresenter, RecycleChanWidgetTemplate) {
  "use strict";

  var RecycleChanWidget = BaseView.extend({
    presenter: PluginPresenter,
    template: RecycleChanWidgetTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return RecycleChanWidget;
});
