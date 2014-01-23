define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/plugins_show"
  ],
  function(BaseView, PluginPresenter, PluginsShowTemplate) {
    "use strict";

    /**
    * Show view for plugins.
    *
    * @class PluginsShow
    * @extends BaseView
    *
    * @constructor
    */
    var PluginsShow = BaseView.extend({
      presenter: PluginPresenter,
      template: PluginsShowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return PluginsShow;
  }
);
