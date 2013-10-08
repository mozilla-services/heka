define(
  [
    "jquery",
    "dygraph",
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_show",
    "presenters/sandbox_presenter",
    "adapters/sandbox_adapter"
  ],
  function($, Dygraph, BaseView, SandboxesShowTemplate, SandboxPresenter, SandboxAdapter) {
    "use strict";

    var SandboxesShow = BaseView.extend({
      template: SandboxesShowTemplate,
      presenter: SandboxPresenter,
      className: "sandboxes-show",

      events: {
        "click .sandbox-graph-legend-control input": "toggleSeries"
      },

      initialize: function() {
        this.sandboxAdapter = new SandboxAdapter(this.model);

        this.listenTo(this.model, "change:data", this.render, this);

        this.sandboxAdapter.fill();
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

      setLegendColors: function(colors) {
        this.$(".sandbox-graph-legend-control label").each(function(i, label) {
          $(label).css({ color: colors[i] });
        });
      }
    });

    return SandboxesShow;
  }
);
