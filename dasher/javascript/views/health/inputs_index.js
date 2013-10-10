define(
  [
    "views/base_view",
    "views/health/inputs_row",
    "hgn!templates/health/inputs_index"
  ],
  function(BaseView, InputsRow, InputsIndexTemplate) {
    "use strict";

    var InputsIndex = BaseView.extend({
      template: InputsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      afterRender: function() {
        this.renderCollection(InputsRow, ".inputs tbody");
      }
    });

    return InputsIndex;
  }
);
