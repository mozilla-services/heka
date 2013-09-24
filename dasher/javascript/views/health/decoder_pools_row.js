define(["views/base_view", "presenters/plugin_presenter", "hgn!templates/health/decoder_pools_row"], function(BaseView, PluginPresenter, DecoderPoolsRowTemplate) {
  "use strict";

  var DecoderPoolsRow = BaseView.extend({
    tagName: "tr",
    presenter: PluginPresenter,
    template: DecoderPoolsRowTemplate,

    initialize: function() {
      this.listenTo(this.model, "change", this.render, this);
    }
  });

  return DecoderPoolsRow;
});
