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

    var SandboxesIndex = BaseView.extend({
      template: SandboxesIndexTemplate,

      initialize: function() {
        this.sandboxesAdapter = new SandboxesAdapter();
        this.collection = this.sandboxesAdapter.sandboxes;

        this.listenTo(this.collection, "add remove reset", this.render, this);

        this.sandboxesAdapter.fill();
      },

      afterRender: function() {
        this.renderCollection(SandboxesRow, "#sandboxes");
      }
    });

    return SandboxesIndex;
  }
);
