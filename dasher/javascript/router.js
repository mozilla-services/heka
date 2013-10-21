define(
  [
    "jquery",
    "backbone",
    "views/health/health_index",
    "views/sandboxes/sandboxes_index",
    "views/sandboxes/sandbox_output_cbuf_show",
    "views/sandboxes/sandbox_output_txt_show"
  ],
  function($, Backbone, HealthIndex, SandboxesIndex, SandboxOutputCbufShow, SandboxOutputTxtShow) {
    "use strict";

    var Router = Backbone.Router.extend({
      routes: {
        "":          "health",    // #
        "health":    "health",    // #health
        "sandboxes": "sandboxes",  // #sandboxes
        "sandboxes/embed/:filename": "embeddedSandboxOutput" // #sandboxes/embed/msgtype_counter.MessageTypeCounts.cbuf
      },

      health: function() {
        this.switch(new HealthIndex());
      },

      sandboxes: function() {
        this.switch(new SandboxesIndex());
      },

      embeddedSandboxOutput: function(filename) {
        var model = new Backbone.Model({ Filename: "data/" + filename });
        var outputView;

        if (filename.match(/\.cbuf$/)) {
          outputView = new SandboxOutputCbufShow({ model: model });
        } else {
          outputView = new SandboxOutputTxtShow({ model: model });
        }

        this.switch(outputView);
      },

      switch: function(view) {
        // Destroy previous view
        if (this.currentView) {
          this.currentView.destroy();
        }

        // Assign new view and render
        this.currentView = view;

        $("#content").html(this.currentView.render().el);

        // Add embed class if the url contains embed
        if (window.location.href.match(/embed/)) {
          $("html").addClass("embed");
        } else {
          $("html").removeClass("embed");
        }
      }
    });

    return Router;
  }
);
