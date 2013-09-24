define(["views/base_view", "presenters/plugin_presenter", "hgn!templates/health/router_widget"], function(BaseView, PluginPresenter, RouterWidgetTemplate) {
  "use strict";

  var RouterWidget = BaseView.extend({
    presenter: PluginPresenter,
    template: RouterWidgetTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return RouterWidget;
});
