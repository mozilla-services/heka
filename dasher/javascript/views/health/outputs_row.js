define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/outputs_row"
  ],
  function(BaseView, PluginPresenter, OutputsRowTemplate) {
    "use strict";

    var OutputsRow = BaseView.extend({
      tagName: "tr",
      presenter: PluginPresenter,
      template: OutputsRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return OutputsRow;
  }
);
