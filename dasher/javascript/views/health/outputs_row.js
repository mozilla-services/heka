define(
  [
    "views/base_view",
    "views/health/coders_row",
    "presenters/plugin_presenter",
    "hgn!templates/health/outputs_row"
  ],
  function(BaseView, CodersRow, PluginPresenter, OutputsRowTemplate) {
    "use strict";

    /**
    * Row view for output plugins.
    *
    * @class OutputsRow
    * @extends BaseView
    *
    * @constructor
    */
    var OutputsRow = BaseView.extend({
      presenter: PluginPresenter,
      template: OutputsRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
        this.CodersRow = CodersRow; // inject for BaseView.renderCoders
      },

      /**
      * Renders Encoder plugins as children of the corresponding Output plugin.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCoders();
      }
    });

    return OutputsRow;
  }
);
