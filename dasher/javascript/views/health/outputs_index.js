define(
  [
    "views/base_view",
    "views/health/outputs_row",
    "hgn!templates/health/outputs_index"
  ],
  function(BaseView, OutputsRow, OutputsIndexTemplate) {
    "use strict";

    var OutputsIndex = BaseView.extend({
      template: OutputsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      afterRender: function() {
        this.renderCollection(OutputsRow, ".outputs tbody");
      }
    });

    return OutputsIndex;
  }
);
