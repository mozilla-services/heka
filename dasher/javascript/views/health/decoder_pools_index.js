define(
  [
    "views/base_view",
    "views/health/decoder_pools_row",
    "hgn!templates/health/decoder_pools_index"
  ],
  function(BaseView, DecodersRow, DecoderPoolsIndexTemplate) {
    "use strict";

    /**
    * Index view for decoder pool plugins.
    *
    * @class DecoderPoolsIndex
    * @extends BaseView
    *
    * @constructor
    */
    var DecoderPoolsIndex = BaseView.extend({
      template: DecoderPoolsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      /**
      * Renders DecodersRow into `.decoder-pools` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(DecodersRow, ".decoder-pools");
      }
    });

    return DecoderPoolsIndex;
  }
);
