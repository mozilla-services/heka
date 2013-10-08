define(
  [
    "views/base_view",
    "views/health/router_widget",
    "views/health/channel_count_widget",
    "hgn!templates/health/globals_index"
  ],
  function(BaseView, RouterWidget, ChannelCountWidget, GlobalsIndexTemplate) {
    "use strict";

    var GlobalsIndex = BaseView.extend({
      template: GlobalsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "reset", this.render, this);
      },

      afterRender: function() {
        // Only render widgets if we have data
        if (this.collection.getRouter()) {
          this.routerWidget = new RouterWidget({ model: this.collection.getRouter() });
          this.routerChannelCountWidget = new ChannelCountWidget({ model: this.collection.getRouter() });
          this.inputRecycleChannelCountWidget = new ChannelCountWidget({ model: this.collection.getInputRecycleChan() });
          this.injectRecycleChannelCountWidget = new ChannelCountWidget({ model: this.collection.getInjectRecycleChan() });

          this.assign(this.routerWidget, "#router-widget");
          this.assign(this.routerChannelCountWidget, "#router-channel-count");
          this.assign(this.inputRecycleChannelCountWidget, "#input-recycle-chan-widget");
          this.assign(this.injectRecycleChannelCountWidget, "#inject-recycle-chan-widget");
        }
      }
    });

    return GlobalsIndex;
  }
);
