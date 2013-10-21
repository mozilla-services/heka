define(
  [
    "jquery",
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandbox_output_cbuf_show",
    "presenters/sandbox_output_cbuf_presenter",
    "adapters/sandbox_output_cbuf_adapter"
  ],
  function($, Dygraph, BaseView, SandboxOutputCbufShowTemplate, SandboxOutputCbufPresenter, SandboxOutputCbufAdapter) {
    "use strict";

    var SandboxOutputCbufShow = BaseView.extend({
      template: SandboxOutputCbufShowTemplate,
      presenter: SandboxOutputCbufPresenter,
      className: "sandboxes-output sandboxes-output-cbuf",

      events: {
        "click .sandbox-graph-legend-control input": "toggleSeries"
      },

      initialize: function() {
        this.adapter = new SandboxOutputCbufAdapter(this.model);

        this.listenTo(this.model, "change:data", this.updateDygraph, this);

        this.adapter.fill();
      },

      toggleSeries: function(event) {
        var $target = $(event.target);

        this.dygraph.setVisibility($target.attr("data-label-id"), $target.is(":checked"));
      },

      render: function() {
        var presentation = this.getPresentation();

        this.$el.html(this.template(presentation));

        if (presentation.data) {
          // Fixes drawing problem in Firefox
          // TODO: Find something better ...
          setTimeout(function() {
            this.dygraph = new Dygraph(this.$(".sandbox-graph")[0], presentation.data, { labels: presentation.labels() });
            this.setLegendColors(this.dygraph.getColors());
          }.bind(this), 10);
        }

        return this;
      },

      updateDygraph: function() {
        // TODO: rerender if there are major changes to the graph
        if (this.dygraph) {
          this.dygraph.updateOptions({ file: this.model.get("data") });
        } else {
          this.render();
        }
      },

      setLegendColors: function(colors) {
        this.$(".sandbox-graph-legend-control label").each(function(i, label) {
          $(label).css({ color: colors[i] });
        });
      }
    });

    return SandboxOutputCbufShow;
  }
);
