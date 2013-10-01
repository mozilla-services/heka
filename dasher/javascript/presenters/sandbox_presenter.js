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
        var labels = _.collect(this.header.column_info, function(column) {
          return column.name + " (" + column.unit + ")";
        });

        labels.unshift("Date");

        return labels;
      }
    });

    return SandboxPresenter;
  }
);
