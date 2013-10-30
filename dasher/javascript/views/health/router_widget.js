define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/router_widget"
  ],
  function(BaseView, PluginPresenter, RouterWidgetTemplate) {
    "use strict";

    /**
    * Widget for router processed count.
    *
    * @class RouterWidget
    * @extends BaseView
    *
    * @constructor
    */
    var RouterWidget = BaseView.extend({
      presenter: PluginPresenter,
      template: RouterWidgetTemplate,

      initialize: function() {
        this.listenTo(this.model, "change:ProcessMessageCount.value", this.render, this);
      }
    });

    return RouterWidget;
  }
);
