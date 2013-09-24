define(
  [
    "views/base_view",
    "views/health/router_widget",
    "views/health/recycle_chan_widget",
    "hgn!templates/health/globals_index"
  ],
  function(BaseView, RouterWidget, RecycleChanWidget, GlobalsIndexTemplate) {
    "use strict";

    var GlobalsIndex = BaseView.extend({
      template: GlobalsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "reset", this.renderSubviews, this);
      },

      renderSubviews: function() {
        this.routerWidget = new RouterWidget({ model: this.collection.getRouter() });
        this.inputRecycleChanWidget = new RecycleChanWidget({ model: this.collection.getInputRecycleChan() });
        this.injectRecycleChanWidget = new RecycleChanWidget({ model: this.collection.getInjectRecycleChan() });

        this.assign(this.routerWidget, "#router-widget");
        this.assign(this.inputRecycleChanWidget, "#input-recycle-chan-widget");
        this.assign(this.injectRecycleChanWidget, "#inject-recycle-chan-widget");
      }
    });

    return GlobalsIndex;
  }
);
