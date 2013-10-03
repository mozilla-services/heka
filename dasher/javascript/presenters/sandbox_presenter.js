define(
  [
    "underscore"
  ],
  function(_) {
    "use strict";

    var SandboxPresenter = function (sandbox) {
      _.extend(this, sandbox.attributes);
    };

    _.extend(SandboxPresenter.prototype, {
      labels: function() {
        var labels = [];

        if (this.header && this.header.column_info) {
          labels.push("Date");

          labels = labels.concat(_.collect(this.header.column_info, function(column) {
            return column.name + " (" + column.unit + ")";
          }));
        }

        return labels;
      },

      // These IDs are meaninful to Dygraph
      legendLabels: function() {
        var labels = this.labels();

        // Remove "Date"
        labels.shift();

        // Start at 1 since we removed zero
        var i = 0;

        var labelsWithID = _.collect(labels, function(label) {
          return { id: i++, name: label };
        });

        return labelsWithID;
      },

      hasHeader: function() {
        return _.has(this, "header");
      },

      secondsPerRow: function() {
        if (this.hasHeader()) {
          return this.header.seconds_per_row;
        }
      },

      numberOfHours: function() {
        if (this.hasHeader()) {
          var secondsInAnHour = 3600;

          var hours = (this.header.seconds_per_row * this.header.rows) / secondsInAnHour;
          var numberOfHours = hours.toString() + " hour";

          if (hours != 1) {
            numberOfHours += "s";
          }

          return numberOfHours;
        }
      }
    });

    return SandboxPresenter;
  }
);
