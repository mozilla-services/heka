define(
  [
    "views/base_view",
    "views/health/router_widget",
    "views/health/channel_count_widget",
    "hgn!templates/health/globals_index"
  ],
  function(BaseView, RouterWidget, ChannelCountWidget, GlobalsIndexTemplate) {
    "use strict";

    /**
    * Index view for global plugins.
    *
    * @class GlobalsIndex
    * @extends BaseView
    *
    * @constructor
    */
    var GlobalsIndex = BaseView.extend({
      template: GlobalsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "reset", this.render, this);
      },

      /**
      * Renders routerWidget, routerChannelCountWidget, inputRecycleChannelCountWidget,
      * injectRecycleChannelCountWidget into corresponding DOM elements after render.
      *
      * Only renders data if the router is available since the first time this is rendered there's
      * no data available.
      *
      * @method afterRender
      */
      afterRender: function() {
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
