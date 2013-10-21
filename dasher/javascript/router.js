define(
  [
    "jquery",
    "backbone",
    "views/health/health_index",
    "views/sandboxes/sandboxes_index"
  ],
  function($, Backbone, HealthIndex, SandboxesIndex) {
    "use strict";

    var Router = Backbone.Router.extend({
      routes: {
        "":          "health",    // #
        "health":    "health",    // #health
        "sandboxes": "sandboxes"  // #sandboxes
      },

      health: function() {
        this.switch(new HealthIndex());
      },

      sandboxes: function() {
        this.switch(new SandboxesIndex());
      },

      switch: function(view) {
        if (this.currentView) {
          this.currentView.destroy();
        }

        this.currentView = view;

        $("#content").html(this.currentView.render().el);
      }
    });

    return Router;
  }
);
