define(["jquery", "backbone", "views/health/health_index"], function($, Backbone, HealthIndex) {
  "use strict";

  var Router = Backbone.Router.extend({
    routes: {
      "":          "health",    // #
      "health":    "health",    // #health
      "sandboxes": "sandboxes"  // #sandboxes
    },

    initialize: function() {
      this.healthIndex = new HealthIndex();

      this.healthIndex.render();
    },

    health: function() {
      this.updateContent(this.healthIndex);
    },

    sandboxes: function() {
      $("#content").html("Sandboxes!");
    },

    updateContent: function(view) {
      $("#content").html(view.el);
    }
  });

  return Router;
});
