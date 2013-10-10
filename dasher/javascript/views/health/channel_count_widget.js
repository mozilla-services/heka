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
        this.listenTo(this.model, "change:InChanLength", this.updateInChanLength, this);
        this.listenTo(this.model, "change:InChanCapacity", this.updateInChanCapacity, this);
      },

      updateInChanLength: function() {
        this.$(".in-chan-length").animateNumbers(this.model.get("InChanLength").value);
      },

      updateInChanCapacity: function() {
        this.$(".in-chan-capacity").animateNumbers(this.model.get("InChanCapacity").value);
      }
    });

    return ChannelCount;
  }
);
