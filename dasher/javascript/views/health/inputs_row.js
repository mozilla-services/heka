define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/inputs_row"
  ],
  function(BaseView, PluginPresenter, InputsRowTemplate) {
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
      tagName: "tr",
      presenter: PluginPresenter,
      template: InputsRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return InputsRow;
  }
);
