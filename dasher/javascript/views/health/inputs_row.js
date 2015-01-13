define(
  [
    "views/base_view",
    "views/health/coders_row",
    "presenters/plugin_presenter",
    "hgn!templates/health/inputs_row"
  ],
  function(BaseView, CodersRow, PluginPresenter, InputsRowTemplate) {
    "use strict";

    /**
    * Row view for input plugins.
    *
    * @class InputsRow
    * @extends BaseView
    *
    * @constructor
    */
    var InputsRow = BaseView.extend({
      presenter: PluginPresenter,
      template: InputsRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
        this.CodersRow = CodersRow; // inject for BaseView.renderCoders
      },

      /**
      * Renders Decoder plugins as children of the corresponding Input plugin.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCoders();
      }
    });

    return InputsRow;
  }
);
