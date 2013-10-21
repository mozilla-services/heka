define(
  [
    "underscore",
    "presenters/sandbox_output_presenter"
  ],
  function(_, SandboxOutputPresenter) {
    "use strict";

    var SandboxOutputCbufPresenter = function (sandboxOutput) {
      _.extend(this, sandboxOutput.attributes);
    };

    _.extend(SandboxOutputCbufPresenter.prototype, SandboxOutputPresenter.prototype, {
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

      timespanDescription: function() {
        if (this.hasHeader()) {
          var secondsInAMinute = 60;
          var secondsInAnHour = 3600;

          var seconds = this.header.seconds_per_row * this.header.rows;
          var numericValue;
          var timespanDescription;

          if (seconds < secondsInAMinute) {
            numericValue = seconds;
            timespanDescription = numericValue.toString() + " second";
          } else if (seconds < secondsInAnHour) {
            numericValue = this.round(seconds / secondsInAMinute);
            timespanDescription = numericValue.toString() + " minute";
          } else {
            numericValue = this.round(seconds / secondsInAnHour);
            timespanDescription = numericValue.toString() + " hour";
          }

          if (numericValue != 1) {
            timespanDescription += "s";
          }

          return timespanDescription;
        }
      },

      round: function(number) {
        return Math.round(number * 100) / 100;
      }
    });

    return SandboxOutputCbufPresenter;
  }
);
