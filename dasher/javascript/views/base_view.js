define(
  [
    "underscore",
    "backbone"
  ],
  function(_, Backbone) {
    "use strict";

    var BaseView = Backbone.View.extend({
      constructor: function(options) {
        this.subviews = [];

        Backbone.View.call(this, options);
      },

      presenter: function(model) {
        return model ? model.attributes : null;
      },

      getPresentation: function() {
        return new this.presenter(this.model);
      },

      render: function() {
        this.destroySubviews();

        var presentation = this.getPresentation();

        this.$el.html(this.template(presentation));

        this.afterRender();

        return this;
      },

      renderCollection: function(itemView, selector) {
        var els = this.collection.collect(function(item) {
          return this.trackSubview(new itemView({ model: item })).render().el;
        }.bind(this));

        this.$(selector).append(els);
      },

      afterRender: function() {
        // Implement in subclasses
      },

      assign: function(view, selector) {
        view.setElement(this.$(selector));
        view.render();

        return this;
      },

      destroy: function() {
        if (this.beforeDestroy) {
          this.beforeDestroy();
        }

        // Turn off adapter polling
        if (this.adapter && this.adapter.stopListeningForUpdates) {
          this.adapter.stopListeningForUpdates();
        }

        this.stopListening();
        this.destroySubviews();
        this.$el.off();
      },

      trackSubview: function(view) {
        if (!_.contains(this.subviews, view)) {
          this.subviews.push(view);
        }

        return view;
      },

      destroySubviews: function() {
        _.invoke(this.subviews, "destroy");
      }
    });

    return BaseView;
  }
);
