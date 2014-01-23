define(
  [
    "views/base_view",
    "presenters/plugin_presenter",
    "hgn!templates/health/filters_row"
  ],
  function(BaseView, PluginPresenter, FiltersRowTemplate) {
    "use strict";

    /**
    * Row view for filter plugins.
    *
    * @class FiltersRow
    * @extends BaseView
    *
    * @constructor
    */
    var FiltersRow = BaseView.extend({
      tagName: "tr",
      presenter: PluginPresenter,
      template: FiltersRowTemplate,

      initialize: function() {
        this.listenTo(this.model, "change", this.render, this);
      }
    });

    return FiltersRow;
  }
);
