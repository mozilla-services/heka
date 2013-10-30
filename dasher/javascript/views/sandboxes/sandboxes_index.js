define(
  [
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_index",
    "adapters/sandboxes_adapter",
    "presenters/plugin_presenter"
  ],
  function(BaseView, SandboxesIndexTemplate, SandboxesAdapter, PluginPresenter) {
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
      presenter: PluginPresenter,
      template: SandboxesIndexTemplate,

      initialize: function() {
        this.adapter = SandboxesAdapter.instance();
        this.collection = this.adapter.sandboxes;

        this.listenTo(this.collection, "add remove reset change", this.render, this);

        this.adapter.fill();
      }
    });

    return SandboxesIndex;
  }
);
