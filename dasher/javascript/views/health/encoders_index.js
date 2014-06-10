define(
  [
    "views/base_view",
    "views/health/encoders_row",
    "hgn!templates/health/encoders_index"
  ],
  function(BaseView, EncodersRow, EncodersIndexTemplate) {
    "use strict";

    /**
    * Index view for Encoder plugins.
    *
    * @class EncodersIndex
    * @extends BaseView
    *
    * @constructor
    */
    var EncodersIndex = BaseView.extend({
      template: EncodersIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      /**
      * Renders EncodersRow into `.encoders tbody` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(EncodersRow, ".encoders tbody");
      }
    });

    return EncodersIndex;
  }
);
