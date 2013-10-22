define(
  [
    "views/base_view",
    "presenters/decoder_pool_plugin_presenter",
    "hgn!templates/health/decoder_pools_row"
  ],
  function(BaseView, DecoderPoolPluginPresenter, DecoderPoolsRowTemplate) {
    "use strict";

    /**
    * Row view for decoder pool plugins.
    *
    * @class DecoderPoolsRow
    * @extends BaseView
    *
    * @constructor
    */
    var DecoderPoolsRow = BaseView.extend({
      tagName: "tbody",
      presenter: DecoderPoolPluginPresenter,
      template: DecoderPoolsRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return DecoderPoolsRow;
  }
);
