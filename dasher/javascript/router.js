define(
  [
    "jquery",
    "backbone",
    "adapters/plugins_adapter",
    "adapters/sandboxes_adapter",
    "views/health/health_index",
    "views/sandboxes/sandboxes_index",
    "views/sandboxes/sandbox_output_cbuf_show",
    "views/sandboxes/sandbox_output_txt_show",
    "views/health/plugins_show",
    "views/termination_report/termination_report_index"
  ],
  function($, Backbone, PluginsAdapter, SandboxesAdapter, HealthIndex, SandboxesIndex, SandboxOutputCbufShow, SandboxOutputTxtShow, PluginsShow, TerminationReportIndex) {
    "use strict";

    /**
    * Router for the dashboard.
    *
    * Provides the following routes:
    *
    * - `/#`
    * - `/#health`
    *
    * - `/#plugins/inputs/:name`
    * - `/#plugins/decoders/:name`
    * - `/#plugins/filters/:name`
    * - `/#plugins/outputs/:name`
    * - `/#plugins/encoders/:name`
    *
    * - `/#sandboxes`
    * - `/#sandboxes/:sandboxName/outputs/:shortFileName`
    * - `/#sandboxes/:sandboxName/outputs/:shortFileName/embed`
    *
    * - `/#termination_report`
    *
    * @class Router
    *
    * @constructor
    */
    var Router = Backbone.Router.extend({
      routes: {
        "": "showHealthIndex",
        "health": "showHealthIndex",

        "plugins/inputs/:name": "showInput",
        "plugins/decoders/:name": "showDecoder",
        "plugins/filters/:name": "showFilter",
        "plugins/outputs/:name": "showOutput",
        "plugins/encoders/:name": "showEncoder",

        "sandboxes": "showSandboxesIndex",
        "sandboxes/:sandboxName/outputs/:shortFileName": "showSandboxOutput",
        "sandboxes/:sandboxName/outputs/:shortFileName/embed": "showSandboxOutput",

        "termination_report": "showTerminationReportIndex"
      },

      /**
      * Loads and navigates to the health index.
      *
      * @method showHealthIndex
      */
      showHealthIndex: function() {
        this._switch(new HealthIndex());
      },

      /**
      * Loads input plugin by name and navigates to plugin's show.
      *
      * @method showInput
      */
      showInput: function(name) {
        PluginsAdapter.instance().findInputWhere({ Name: name }, function(input) {
          this._switch(new PluginsShow({ model: input }));
        }.bind(this));
      },

      /**
      * Loads decoder plugin by name and navigates to plugin's show.
      *
      * @method showDecoder
      */
      showDecoder: function(name) {
        PluginsAdapter.instance().findDecoderWhere({ Name: name }, function(decoder) {
          this._switch(new PluginsShow({ model: decoder }));
        }.bind(this));
      },

      /**
      * Loads filter plugin by name and navigates to plugin's show.
      *
      * @method showInput
      */
      showFilter: function(name) {
        PluginsAdapter.instance().findFilterWhere({ Name: name }, function(filter) {
          this._switch(new PluginsShow({ model: filter }));
        }.bind(this));
      },

      /**
      * Loads output plugin by name and navigates to plugin's show.
      *
      * @method showInput
      */
      showOutput: function(name) {
        PluginsAdapter.instance().findOutputWhere({ Name: name }, function(output) {
          this._switch(new PluginsShow({ model: output }));
        }.bind(this));
      },

      /**
      * Loads encoder plugin by name and navigates to plugin's show.
      *
      * @method showEncoder
      */
      showEncoder: function(name) {
        PluginsAdapter.instance().findEncoderWhere({ Name: name }, function(encoder) {
          this._switch(new PluginsShow({ model: encoder }));
        }.bind(this));
      },

      /**
      * Loads and navigates to the sandboxes index.
      *
      * @method showSandboxesIndex
      */
      showSandboxesIndex: function() {
        this._switch(new SandboxesIndex());
      },

      /**
      * Loads the correct sandbox output show view based on the Filename extension.
      *
      * @method showSandboxOutput
      */
      showSandboxOutput: function(sandboxName, shortFilename) {
        SandboxesAdapter.instance().findSandboxWhere({ Name: sandboxName }, function(sandbox) {
          var sandboxOutput = sandbox.findOutputByShortFilename(shortFilename);

          if (sandboxOutput) {
            var outputView;

            if (shortFilename.match(/\.cbuf$/)) {
              outputView = new SandboxOutputCbufShow({ model: sandboxOutput, sandbox: sandbox });
            } else {
              outputView = new SandboxOutputTxtShow({ model: sandboxOutput, sandbox: sandbox });
            }

            this._switch(outputView);
          }
        }.bind(this));
      },

      /**
      * Loads and navigates to the termination report index.
      *
      * @method showTerminationReportIndex
      */
      showTerminationReportIndex: function() {
        this._switch(new TerminationReportIndex());
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

        // Assign new view
        this.currentView = view;

        // Add embed class if the url contains embed
        if (window.location.href.match(/\/embed$/)) {
          $("html").addClass("embed");
        } else {
          $("html").removeClass("embed");
        }

        // Make sure we're scrolled to the top
        window.scrollTo(0, 1);

        // Render and insert the element
        $("#content").html(this.currentView.render().el);

        // Close navbars
        if ($(window).width() <= 768 && $(".navbar-collapse").is(":visible")) {
          $(".navbar-collapse").collapse("hide");
        }
      }
    });

    return Router;
  }
);
