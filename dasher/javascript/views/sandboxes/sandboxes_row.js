define(
  [
    "jquery",
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_row",
    "presenters/sandbox_presenter",
    "views/sandboxes/sandbox_output_cbuf_show",
    "views/sandboxes/sandbox_output_txt_show"
  ],
  function($, Dygraph, BaseView, SandboxesRowTemplate, SandboxPresenter, SandboxOutputCbufShow, SandboxOutputTxtShow) {
    "use strict";

    var SandboxesRow = BaseView.extend({
      template: SandboxesRowTemplate,
      presenter: SandboxPresenter,
      className: "sandboxes-row",

      initialize: function() {
        //this.sandboxAdapter = new SandboxAdapter(this.model);

        //this.listenTo(this.model, "change:Outputs", this.render, this);

        //this.sandboxAdapter.fill();
      },

      afterRender: function() {
        var els = this.model.get("Outputs").collect(function(output) {
          var subview;

          if (output.get("Filename").match(/\.cbuf$/)) {
            subview = new SandboxOutputCbufShow({ model: output });
          } else {
            subview = new SandboxOutputTxtShow({ model: output });
          }

          return this.trackSubview(subview).render().el;
        }.bind(this));

        this.$("div.sandbox-outputs").append(els);
      }
    });

    return SandboxesRow;
  }
);
