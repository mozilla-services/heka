define(
  [
    "jquery",
    "underscore",
    "models/report",
    "collections/reports",
    "views/base_view",
    "views/reports_row",
    "views/router_widget",
    "views/recycle_chan_widget",
    "hgn!templates/reports_index"
  ],
  function($, _, Report, Reports, BaseView, ReportsRow, RouterWidget, RecycleChanWidget, ReportsIndexTemplate) {
    "use strict";

    var ReportsIndex = BaseView.extend({
      initialize: function() {
        this.collection = new Reports();

        // Collection events
        this.listenTo(this.collection, "add remove", this.render, this);

        // Subviews
        this.routerWidget;
        this.inputRecycleChanWidget;
        this.injectRecycleChanWidget;

        // Load initial data
        this.fetchReports();

        // Update reports every 5 seconds
        this.reportsUpdateInterval = setInterval(_.bind(this.fetchReports, this), 5000);
      },

      // TODO: Reconsider function binding
      fetchReports: function() {
        console.log("fetching reports...");

        $.getJSON("sample_data/heka_report.json").then(_.bind(function(response) {
          var reports = _.collect(response.reports, function(report) {
            return new Report(_.extend(report, { id: report.Plugin }));
          });

          // If collection has items then merge in changes otherwise reset
          if (this.collection.length > 0) {
            this.collection.set(reports);
          } else {
            this.collection.reset(reports);

            this.routerWidget = new RouterWidget({ model: this.collection.getRouter() });
            this.inputRecycleChanWidget = new RecycleChanWidget({ model: this.collection.getInputRecycleChan() });
            this.injectRecycleChanWidget = new RecycleChanWidget({ model: this.collection.getInjectRecycleChan() });

            this.render();
          }
        }, this));
      },

      render: function() {
        console.log("render index");

        this.$el.html(this.template());

        // Render tables rows
        var els = this.collection.collect(function(report) {
          return new ReportsRow({ model: report }).render().el;
        });

        this.$("table.reports tbody").append(els);

        // Render widgets
        if (this.routerWidget) {
          this.$("#router-widget").append(this.routerWidget.render().el);
        }

        if (this.inputRecycleChanWidget) {
          this.$("#input-recycle-chan-widget").append(this.inputRecycleChanWidget.render().el);
        }

        if (this.injectRecycleChanWidget) {
          this.$("#inject-recycle-chan-widget").append(this.injectRecycleChanWidget.render().el);
        }

        //this.$('.round-chart').easyPieChart();

        return this;
      },

      template: ReportsIndexTemplate
    });

    return ReportsIndex;
  }
);
