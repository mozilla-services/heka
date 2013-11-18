define(
  [
    "views/base_view",
    "views/health/inputs_row",
    "hgn!templates/health/inputs_index"
  ],
  function(BaseView, InputsRow, InputsIndexTemplate) {
    "use strict";

    /**
    * Index view for input plugins.
    *
    * @class InputsIndex
    * @extends BaseView
    *
    * @constructor
    */
    var InputsIndex = BaseView.extend({
      template: InputsIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      /**
      * Renders InputsRow into `.inputs tbody` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(InputsRow, ".inputs tbody");
      }
    });

    return InputsIndex;
  }
);
