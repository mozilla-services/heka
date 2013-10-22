define(
  [
    "jquery",
    "backbone",
    "views/health/health_index",
    "views/sandboxes/sandboxes_index",
    "views/sandboxes/sandbox_output_cbuf_show",
    "views/sandboxes/sandbox_output_txt_show"
  ],
  function($, Backbone, HealthIndex, SandboxesIndex, SandboxOutputCbufShow, SandboxOutputTxtShow) {
    "use strict";

    /**
    * Router for the dashboard.
    *
    * Provides the following routes:
    *
    * - \#
    * - \#health
    * - \#sandboxes
    * - \#sandboxes/embed/:filename
    *
    * @class Router
    *
    * @constructor
    */
    var Router = Backbone.Router.extend({
      routes: {
        "":          "health",    // #
        "health":    "health",    // #health
        "sandboxes": "sandboxes",  // #sandboxes
        "sandboxes/embed/:filename": "embeddedSandboxOutput" // #sandboxes/embed/msgtype_counter.MessageTypeCounts.cbuf
      },

      /**
      * Loads and navigates to the health index.
      *
      * @method health
      */
      health: function() {
        this._switch(new HealthIndex());
      },

      /**
      * Loads and navigates to the sandboxes index.
      *
      * @method sandboxes
      */
      sandboxes: function() {
        this._switch(new SandboxesIndex());
      },

      /**
      * Loads the correct sandbox output show view based the Filename extension. These views
      * stand-alone without navigation and are used for embedding.
      *
      * @method embeddedSandboxOutput
      */
      embeddedSandboxOutput: function(filename) {
        var model = new Backbone.Model({ Filename: "data/" + filename });
        var outputView;

        if (filename.match(/\.cbuf$/)) {
          outputView = new SandboxOutputCbufShow({ model: model });
        } else {
          outputView = new SandboxOutputTxtShow({ model: model });
        }

        this._switch(outputView);
      },

      /**
      * Destroys the previous view and switches to the new one.
      *
      * @method _switch
      * @private
      */
      _switch: function(view) {
        // Destroy previous view
        if (this.currentView) {
          this.currentView.destroy();
        }

        // Assign new view and render
        this.currentView = view;

        $("#content").html(this.currentView.render().el);

        // Add embed class if the url contains embed
        if (window.location.href.match(/embed/)) {
          $("html").addClass("embed");
        } else {
          $("html").removeClass("embed");
        }
      }
    });

    return Router;
  }
);
