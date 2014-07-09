define(
  [
    "views/base_view",
    "views/health/decoders_row",
    "hgn!templates/health/decoders_index"
  ],
  function(BaseView, DecodersRow, DecodersIndexTemplate) {
    "use strict";

    /**
    * Index view for decoder plugins.
    *
    * @class DecodersIndex
    * @extends BaseView
    *
    * @constructor
    */
    var DecodersIndex = BaseView.extend({
      template: DecodersIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      /**
      * Renders DecodersRow into `.decoders tbody` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(DecodersRow, ".decoders tbody");
      }
    });

    return DecodersIndex;
  }
);
