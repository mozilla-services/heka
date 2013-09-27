define(["jquery", "jquery.animateNumbers", "views/base_view", "presenters/plugin_presenter", "hgn!templates/health/router_widget"], function($, animateNumbers, BaseView, PluginPresenter, RouterWidgetTemplate) {
  "use strict";

  var RouterWidget = BaseView.extend({
    presenter: PluginPresenter,
    template: RouterWidgetTemplate,

    initialize: function() {
      this.listenTo(this.model, "change:ProcessMessageCount", this.updateProcessMessageCount, this);
    },

    updateProcessMessageCount: function() {
      this.$("h4 strong").animateNumbers(this.model.get("ProcessMessageCount").value);
    }
  });

  return RouterWidget;
});
