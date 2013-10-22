define(
  [
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_index",
    "models/sandbox",
    "adapters/sandboxes_adapter",
    "views/sandboxes/sandboxes_row"
  ],
  function(BaseView, SandboxesIndexTemplate, Sandbox, SandboxesAdapter, SandboxesRow) {
    "use strict";

    /**
    * Index view for sandboxes. This is a top level view that's loaded by the router.
    *
    * @class SandboxesIndex
    * @extends BaseView
    *
    * @constructor
    */
    var SandboxesIndex = BaseView.extend({
      template: SandboxesIndexTemplate,

      initialize: function() {
        this.adapter = new SandboxesAdapter();
        this.collection = this.adapter.sandboxes;

        this.listenTo(this.collection, "add remove reset", this.render, this);

        this.adapter.fill();
      },

      /**
      * Renders SandboxesRow into #sandboxes after render.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.renderCollection(SandboxesRow, "#sandboxes");
      }
    });

    return SandboxesIndex;
  }
);
