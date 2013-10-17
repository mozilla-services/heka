define(
  [
    "jquery",
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandbox_output_txt_show",
    "adapters/sandbox_output_txt_adapter"
  ],
  function($, Dygraph, BaseView, SandboxOutputTxtShowTemplate, SandboxOutputTxtAdapter) {
    "use strict";

    var SandboxOutputTxtShow = BaseView.extend({
      template: SandboxOutputTxtShowTemplate,
      className: "sandboxes-output-txt",

      initialize: function() {
        this.adapter = new SandboxOutputTxtAdapter(this.model);

        this.listenTo(this.model, "change:data", this.render, this);

        this.adapter.fill();
      }
    });

    return SandboxOutputTxtShow;
  }
);
