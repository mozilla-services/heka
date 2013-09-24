define(
  [
    "views/base_view",
    "views/health/decoder_pools_row",
    "hgn!templates/health/decoder_pools_index"
  ],
  function(BaseView, DecodersRow, DecoderPoolsIndexTemplate) {
    "use strict";

    var DecoderPoolsIndex = BaseView.extend({
      template: DecoderPoolsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "reset", this.render, this);
      },

      afterRender: function() {
        this.renderCollection(DecodersRow, ".decoder-pools tbody");
      }
    });

    return DecoderPoolsIndex;
  }
);
