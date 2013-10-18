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
        this.leaveSubviews();

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

      leave: function() {
        if (this.beforeLeave) {
          this.beforeLeave();
        }

        // Turn off adapter polling
        if (this.adapter && this.adapter.stopListeningForUpdates) {
          this.adapter.stopListeningForUpdates();
        }

        this.stopListening();
        this.leaveSubviews();
        this.$el.off();
      },

      trackSubview: function(view) {
        if (!_.contains(this.subviews, view)) {
          this.subviews.push(view);
        }

        return view;
      },

      leaveSubviews: function() {
        _.invoke(this.subviews, "leave");
      }
    });

    return BaseView;
  }
);
