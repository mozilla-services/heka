define(
  [
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_index",
    "models/sandbox",
    "adapters/sandbox_list_adapter",
    "views/sandboxes/sandboxes_show"
  ],
  function(BaseView, SandboxesIndexTemplate, Sandbox, SandboxListAdapter, SandboxesShow) {
    "use strict";

    var SandboxesIndex = BaseView.extend({
      template: SandboxesIndexTemplate,

      initialize: function() {
        this.sandboxListAdapter = new SandboxListAdapter();
        this.collection = this.sandboxListAdapter.sandboxes;

        this.listenTo(this.collection, "reset", this.render, this);

        this.sandboxListAdapter.fill();
      },

      afterRender: function() {
        this.renderCollection(SandboxesShow, "#sandboxes");
      }
    });

    return SandboxesIndex;
  }
);
