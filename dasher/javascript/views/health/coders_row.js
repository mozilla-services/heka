define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/coders_row"
  ],
  function(BaseView, PluginPresenter, CodersRowTemplate) {
    "use strict";

    /**
    * Row view for Coders plugins.
    *
    * @class CodersRow
    * @extends BaseView
    *
    * @constructor
    */
    var CodersRow = BaseView.extend({
      presenter: PluginPresenter,
      template: CodersRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return CodersRow;
  }
);
