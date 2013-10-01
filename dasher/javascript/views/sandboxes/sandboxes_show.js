define(
  [
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_show",
    "presenters/sandbox_presenter",
    "adapters/sandbox_adapter"
  ],
  function(Dygraph, BaseView, SandboxesShowTemplate, SandboxPresenter, SandboxAdapter) {
    "use strict";

    var SandboxesShow = BaseView.extend({
      template: SandboxesShowTemplate,
      presenter: SandboxPresenter,
      className: "sandboxes-show",

      initialize: function() {
        this.sandboxAdapter = new SandboxAdapter(this.model);

        this.listenTo(this.model, "change:data", this.setupGraph, this);

        this.sandboxAdapter.fill();
      },

      setupGraph: function() {
        var presentation = this.getPresentation();

        this.dygraph = new Dygraph(this.$(".sandbox-graph")[0], presentation.data, { labels: presentation.labels() });
      }
    });

    return SandboxesShow;
  }
);
