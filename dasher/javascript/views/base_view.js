define(
  [
    "backbone"
  ],
  function(Backbone) {
    "use strict";

    var BaseView = Backbone.View.extend({
      presenter: function(model) {
        return model ? model.attributes : null;
      },

      getPresentation: function() {
        return new this.presenter(this.model);
      },

      render: function() {
        console.log("render", this);

        var presentation = this.getPresentation();

        this.$el.html(this.template(presentation));

        this.afterRender();

        return this;
      },

      renderCollection: function(itemView, selector) {
        var els = this.collection.collect(function(item) {
          return new itemView({ model: item }).render().el;
        });

        this.$(selector).append(els);
      },

      afterRender: function() {
        // Implement in subclasses
      },

      assign: function(view, selector) {
        view.setElement(this.$(selector)).render();

        return this;
      }
    });

    return BaseView;
  }
);
