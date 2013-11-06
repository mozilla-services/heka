define(
  [
    "underscore",
    "backbone"
  ],
  function(_, Backbone) {
    "use strict";

    /**
    * Base class for views that provides common rendering, model presentation, DOM assignment,
    * subview tracking, and tear-down.
    *
    * @class BaseView
    *
    * @constructor
    *
    * @param {Object} options configuration options passed along to Backbone.View
    */
    var BaseView = Backbone.View.extend({
      constructor: function(options) {
        this.subviews = [];

        Backbone.View.call(this, options);
      },

      /**
      * Basic object presenter using model's attributes. This should be overridden by subclasses
      * wishing to provide custom presentation.
      *
      * @property {Function} presenter
      */
      presenter: function(model) {
        return model ? model.attributes : null;
      },

      /**
      * Applies configured presenter to local model or collection.
      *
      * @method getPresentation
      * @return {Object}
      */
      getPresentation: function() {
        var presentation;

        if (this.model) {
          presentation = new this.presenter(this.model);
        } else if (this.collection) {
          var presentedCollection = this.collection.collect(function(i) {
            return new this.presenter(i);
          }.bind(this));

          presentation = { collection: presentedCollection };
        }

        return presentation;
      },

      /**
      * Renders by combining template and presentation and inserting into the associated element.
      *
      * @method render
      * @return {BaseView} this
      * @chainable
      */
      render: function() {
        this.destroySubviews();

        var presentation = this.getPresentation();

        this.$el.html(this.template(presentation));

        this.afterRender();

        return this;
      },

      /**
      * Called after render completes. Provides easy access to custom rendering for subclasses
      * without having to override render.
      *
      * @method afterRender
      */
      afterRender: function() {
        // Implement in subclasses
      },

      /**
      * Renders local collection using the provided view and inserts into the provided selector.
      *
      * @method renderCollection
      * @param {Backbone.View} itemView view for rendering each item in the collection
      * @param {String} selector jQuery selector to insert the collected elements
      */
      renderCollection: function(itemView, selector) {
        var els = this.collection.collect(function(item) {
          return this.trackSubview(new itemView({ model: item })).render().el;
        }.bind(this));

        this.$(selector).append(els);
      },

      /**
      * Assigns view to a selector.
      *
      * @method assign
      * @param {Backbone.View} view to assign
      * @param {String} selector jQuery selector for the element to be assigned
      * @return {BaseView} this
      */
      assign: function(view, selector) {
        view.setElement(this.$(selector));
        view.render();
      },

      /**
      * Destroys view by stopping Backbone event listeners, disabling jQuery events, and destroying
      * subviews.
      *
      * @method destroy
      */
      destroy: function() {
        if (this.beforeDestroy) {
          this.beforeDestroy();
        }

        this.stopListening();
        this.destroySubviews();
        this.$el.off();
      },

      /**
      * Keeps track of a subview so that it can later be destroyed.
      *
      * @method trackSubview
      * @param {BaseView} view to track
      * @return {BaseView} tracked view
      */
      trackSubview: function(view) {
        if (!_.contains(this.subviews, view)) {
          this.subviews.push(view);
        }

        return view;
      },

      /**
      * Destroys all subviews.
      *
      * @method destroySubviews
      */
      destroySubviews: function() {
        _.invoke(this.subviews, "destroy");

        this.subviews = [];
      }
    });

    return BaseView;
  }
);
