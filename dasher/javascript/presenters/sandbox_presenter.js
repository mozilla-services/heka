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
      }
    });

    return SandboxPresenter;
  }
);
