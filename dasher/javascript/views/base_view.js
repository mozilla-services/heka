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

      instantiated: false,

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
      * Input and output plugins may have, respectively, decoder and encoder
      * plugins consuming and feeding them. This method renders the
      * appropriate coders.
      *
      * @method renderCoders
      */
      renderCoders: function() {
        var collection = this.model.get('coders');
        if (!collection) {
          return;
        }

        this.$(".coders_container").empty()
        collection.forEach(function(item) {
          // We assume that *puts have unique names, and that *coder names
          // begin with the name of the *put they work with.
          if (item.id.indexOf(this.model.id) == 0)  {
            var view = new this.CodersRow({model: item});
            this.trackSubview(view);
            this.$(".coders_container").append(view.render().el);
          }
        }.bind(this))
      },

      loadCollapseState: function() {
        var cook = $.cookie('collapseState');
        if (typeof cook == "undefined") {
          cook = {};
        }
        return cook;
      },

      storeCollapseState: function(k, v) {
        var collapseState = this.loadCollapseState();
        collapseState[k] = v;
        $.cookie('collapseState', collapseState);
      },

      clearCollapseState: function(k) {
        var collapseState = this.loadCollapseState();
        delete collapseState[k];
        $.cookie('collapseState', collapseState);
      },

      /**
      * Manually manage state for coder collapsers. Must be idempotent.
      * Includes initialization routines that have to happen after children
      * have rendered. Called by Input/OutputIndex.
      */
      manageCollapseState: function(rowContainerSelector) {
        // Don't use "this." in any stateful way; keep state in the cookie.

        if (!BaseView.instantiated) {
          BaseView.instantiated = true;
          $.cookie.json = true;
          // https://github.com/twbs/bootstrap/issues/14282#issuecomment-51553141
          $.fn.collapse.Constructor.DEFAULTS.toggle = false;
          if (this.checkForDecoders) { // InputsIndex
            this.checkForDecoders();
          }
        }

        // setTimeout is for the Firefox drawing bug.
        var eachState, div, anchor;
        var collapseState = this.loadCollapseState();

        setTimeout(function() {
          $(".panel-collapse").each(function(i, el) {
            $(this).removeClass('in'); // Closes.
          });
          for (var divId in collapseState) {
            eachState = collapseState[divId];
            div = $('#'+divId);
            if (div) {
              div.collapse(eachState);
              // XXX have to manually set twirly glyph per main.css
              anchor = div.parent().find(".accordion-toggle")
              $(anchor).removeClass("collapsed");
            }
          }
        }, 10);

        // Bind cookie updates to show and hide.
        $(rowContainerSelector).on('shown.bs.collapse', function (event) {
          var divId = event.target.id;
          this.storeCollapseState(divId, 'show');
        }.bind(this));

        $(rowContainerSelector).on('hidden.bs.collapse', function (event) {
          var divId = event.target.id;
          this.clearCollapseState(divId);
        }.bind(this));

        // Finally, schedule a call to prevent slow drift open.
        setTimeout(this.manageCollapseState.bind(this), 30*1000);
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
