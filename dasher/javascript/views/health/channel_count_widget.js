define(["views/base_view", "presenters/plugin_presenter", "hgn!templates/health/channel_count_widget"], function(BaseView, PluginPresenter, ChannelCountTemplate) {
  "use strict";

  var ChannelCount = BaseView.extend({
    presenter: PluginPresenter,
    template: ChannelCountTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return ChannelCount;
});
