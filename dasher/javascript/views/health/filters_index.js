define(
  [
    "views/base_view",
    "views/health/filters_row",
    "hgn!templates/health/filters_index"
  ],
  function(BaseView, FiltersRow, FiltersIndexTemplate) {
    "use strict";

    var FiltersIndex = BaseView.extend({
      template: FiltersIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "reset", this.render, this);
      },

      afterRender: function() {
        this.renderCollection(FiltersRow, ".filters tbody");
      }
    });

    return FiltersIndex;
  }
);
