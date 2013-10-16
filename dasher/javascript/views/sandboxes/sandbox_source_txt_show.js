define(
  [
    "jquery",
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandbox_source_txt_show",
    "adapters/sandbox_source_txt_adapter"
  ],
  function($, Dygraph, BaseView, SanboxSourceTxtShowTemplate, SandboxSourceTxtAdapter) {
    "use strict";

    var SanboxSourceTxtShow = BaseView.extend({
      template: SanboxSourceTxtShowTemplate,
      className: "sandboxes-source-txt",

      initialize: function() {
        this.adapter = new SandboxSourceTxtAdapter(this.model);

        this.listenTo(this.model, "change:data", this.render, this);

        this.adapter.fill();
      }
    });

    return SanboxSourceTxtShow;
  }
);
