define(
  [
    "views/base_view",
    "views/health/filters_row",
    "hgn!templates/health/filters_index"
  ],
  function(BaseView, FiltersRow, FiltersIndexTemplate) {
    "use strict";

    /**
    * Index view for filter plugins.
    *
    * @class FiltersIndex
    * @extends BaseView
    *
    * @constructor
    */
    var FiltersIndex = BaseView.extend({
      template: FiltersIndexTemplate,

      initialize: function() {
        this.listenTo(this.collection, "add remove reset", this.render, this);
      },

      /**
      * Renders FiltersRow into `.filters_row_container` after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(FiltersRow, ".filters_row_container");
      }
    });

    return FiltersIndex;
  }
);
