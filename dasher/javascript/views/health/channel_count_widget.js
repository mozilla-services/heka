define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/channel_count_widget"
  ],
  function(BaseView, PluginPresenter, ChannelCountTemplate) {
    "use strict";

    var ChannelCount = BaseView.extend({
      presenter: PluginPresenter,
      template: ChannelCountTemplate,

      initialize: function() {
        this.listenTo(this.model, "change:InChanLength.value", this.render, this);
        this.listenTo(this.model, "change:InChanCapacity.value", this.render, this);
      },

      updateInChanLength: function() {
        this.$(".in-chan-length").html(this.model.get("InChanLength.value"));
      },

      updateInChanCapacity: function() {
        this.$(".in-chan-capacity").html(this.model.get("InChanCapacity.value"));
      }
    });

    return ChannelCount;
  }
);
