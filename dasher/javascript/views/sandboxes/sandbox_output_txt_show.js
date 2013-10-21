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
      className: "sandboxes-output sandboxes-output-txt",

      initialize: function() {
        this.adapter = new SandboxOutputTxtAdapter(this.model);

        this.listenTo(this.model, "change:Name", this.updateName, this);
        this.listenTo(this.model, "change:data", this.updateData, this);

        this.adapter.fill();
      },

      updateName: function() {
        this.$("h4").html(this.model.get("Name"));
      },

      updateData: function() {
        this.$("pre").html(this.model.get("data"));
      }
    });

    return SandboxOutputTxtShow;
  }
);
