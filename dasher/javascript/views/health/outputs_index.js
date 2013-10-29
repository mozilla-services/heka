define(
  [
    "views/base_view",
    "views/health/outputs_row",
    "hgn!templates/health/outputs_index"
  ],
  function(BaseView, OutputsRow, OutputsIndexTemplate) {
    "use strict";

    /**
    * Index view for output plugins.
    *
    * @class OutputsIndex
    * @extends BaseView
    *
    * @constructor
    */
    var OutputsIndex = BaseView.extend({
      template: OutputsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      /**
      * Renders OutputsRow into `.outputs tbody` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(OutputsRow, ".outputs tbody");
      }
    });

    return OutputsIndex;
  }
);
