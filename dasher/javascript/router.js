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

      initialize: function() {
        this.healthIndex = new HealthIndex();
        this.sandboxesIndex = new SandboxesIndex();

        this.healthIndex.render();
        this.sandboxesIndex.render();
      },

      health: function() {
        this.updateContent(this.healthIndex);
      },

      sandboxes: function() {
        this.updateContent(this.sandboxesIndex);
      },

      updateContent: function(view) {
        $("#content").html(view.el);
      }
    });

    return Router;
  }
);
