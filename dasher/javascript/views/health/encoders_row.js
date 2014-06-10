define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/encoders_row"
  ],
  function(BaseView, PluginPresenter, EncodersRowTemplate) {
    "use strict";

    /**
    * Row view for encoders plugins.
    *
    * @class EncodersRow
    * @extends BaseView
    *
    * @constructor
    */
    var EncodersRow = BaseView.extend({
      tagName: "tr",
      presenter: PluginPresenter,
      template: EncodersRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return EncodersRow;
  }
);
