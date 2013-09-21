define(["backbone", "models/report"], function(Backbone, Report) {
  "use strict";

  var Reports = Backbone.Collection.extend({
    model: Report,

    getRouter: function() {
      return this.findWhere({ id: "Router" });
    },

    getInputRecycleChan: function() {
      return this.findWhere({ id: "inputRecycleChan" });
    },

    getInjectRecycleChan: function() {
      return this.findWhere({ id: "injectRecycleChan" });
    }
  });

  return Reports;
});
