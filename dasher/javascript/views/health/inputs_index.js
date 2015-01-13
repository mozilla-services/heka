define(
  [
    "jquery",
    "views/base_view",
    "views/health/inputs_row",
    "hgn!templates/health/inputs_index"
  ],
  function($, BaseView, InputsRow, InputsIndexTemplate) {
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
        this.checkCount = 0;
      },

      // Call on load to expand rows that have decoders
      checkForDecoders: function() {
         // Startup race condition
        if (this.collection.length == 0 && this.checkCount > 50) {
          this.checkout += 1;
          setTimeout(this.checkForDecoders.bind(this), 100);
          return;
        }

        this.collection.forEach(function(input) {
          var divId;
          if (input.get("coders") && input.get("coders").length) {
            divId = "inputCollapseListGroup-" + input.get("id");
            this.storeCollapseState(divId, 'show');
          }
        }.bind(this));

      },

      /**
      * Renders InputsRow into `.inputs_row_container` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(InputsRow, ".inputs_row_container");
        this.manageCollapseState(".inputs_row_container");
      }
    });

    return InputsIndex;
  }
);
