define(["views/base_view", "presenters/plugin_presenter", "hgn!templates/health/plugins_row"], function(BaseView, PluginPresenter, PluginsRowTemplate) {
  "use strict";

  var PluginsRow = BaseView.extend({
    tagName: "tr",
    presenter: PluginPresenter,
    template: PluginsRowTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return PluginsRow;
});
