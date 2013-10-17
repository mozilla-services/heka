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
        if (!this.healthIndex) {
          this.healthIndex = new HealthIndex();
        }

        this.updateContent(this.healthIndex);
      },

      sandboxes: function() {
        if (!this.sandboxesIndex) {
          this.sandboxesIndex = new SandboxesIndex();
        }

        this.updateContent(this.sandboxesIndex);
      },

      updateContent: function(view) {
        $("#content").html(view.render().el);
      }
    });

    return Router;
  }
);
