define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/channel_count_widget"
  ],
  function(BaseView, PluginPresenter, ChannelCountWidgetTemplate) {
    "use strict";

    /**
    * Widget for displaying channel counts.
    *
    * @class ChannelCountWidget
    * @extends BaseView
    *
    * @constructor
    */
    var ChannelCountWidget = BaseView.extend({
      presenter: PluginPresenter,
      template: ChannelCountWidgetTemplate,

      initialize: function() {
        this.listenTo(this.model, "change:InChanLength.value", this.render, this);
        this.listenTo(this.model, "change:InChanCapacity.value", this.render, this);
      },

      /**
      * Efficiently updates the in channel length.
      *
      * @method updateInChanLength
      */
      updateInChanLength: function() {
        this.$(".in-chan-length").html(this.model.get("InChanLength.value"));
      },

      /**
      * Efficiently updates the in channel capacity.
      *
      * @method updateInChanCapacity
      */
      updateInChanCapacity: function() {
        this.$(".in-chan-capacity").html(this.model.get("InChanCapacity.value"));
      }
    });

    return ChannelCountWidget;
  }
);
