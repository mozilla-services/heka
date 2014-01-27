define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/decoders_row"
  ],
  function(BaseView, PluginPresenter, DecodersRowTemplate) {
    "use strict";

    /**
    * Row view for decoders plugins.
    *
    * @class DecodersRow
    * @extends BaseView
    *
    * @constructor
    */
    var DecodersRow = BaseView.extend({
      tagName: "tr",
      presenter: PluginPresenter,
      template: DecodersRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return DecodersRow;
  }
);
