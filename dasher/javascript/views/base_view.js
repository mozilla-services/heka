define(["backbone"], function(Backbone) {
  "use strict";

  var BaseView = Backbone.View.extend({
    presenter: function(model) {
      return model ? model.attributes : null;
    },

    render: function() {
      console.log("render");

      var context = new this.presenter(this.model);

      this.$el.html(this.template(context));

      this.afterRender();

      return this;
    },

    afterRender: function() {
      // Implement in subclasses
    }
  });

  return BaseView;
});
