define(["jquery", "backbone", "views/reports_index"], function($, Backbone, ReportsIndex) {
  "use strict";

  var Router = Backbone.Router.extend({
    routes: {
      "":          "health",    // #
      "health":    "health",    // #health
      "sandboxes": "sandboxes"  // #sandboxes
    },

    initialize: function() {
      this.reportsIndex = new ReportsIndex();

      this.reportsIndex.render();
    },

    health: function() {
      this.updateContent(this.reportsIndex);
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
