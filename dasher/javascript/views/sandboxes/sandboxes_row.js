define(
  [
    "jquery",
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_row",
    "presenters/sandbox_presenter",
    "views/sandboxes/sandbox_source_cbuf_show",
    "views/sandboxes/sandbox_source_txt_show"
  ],
  function($, Dygraph, BaseView, SandboxesRowTemplate, SandboxPresenter, SandboxSourceCbufShow, SandboxSourceTxtShow) {
    "use strict";

    var SandboxesRow = BaseView.extend({
      template: SandboxesRowTemplate,
      presenter: SandboxPresenter,
      className: "sandboxes-row",

      initialize: function() {
        console.log("I AM SANDBOX ROW HEAR ME ROAR")
        //this.sandboxAdapter = new SandboxAdapter(this.model);

        //this.listenTo(this.model, "change:Outputs", this.render, this);

        //this.sandboxAdapter.fill();
      },

      afterRender: function() {
        console.log("AFTER RENDER", this.model);

        var els = this.model.get("Outputs").collect(function(output) {
          var subview;

          console.log("OUTPUT", output);

          if (output.get("Filename").match(/\.cbuf$/)) {
            subview = new SandboxSourceCbufShow({ model: output });
          } else {
            subview = new SandboxSourceTxtShow({ model: output });
          }

          return this.trackSubview(subview).render().el;
        }.bind(this));

        this.$("div.sandbox-outputs").append(els);
      }
    });

    return SandboxesRow;
  }
);
