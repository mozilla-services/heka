define(
  [
    "underscore",
    "presenters/sandbox_output_presenter"
  ],
  function(_, SandboxOutputPresenter) {
    "use strict";

    /**
    * Presents a circular buffer SandboxOutput for use in a view.
    *
    * @class SandboxOutputCbufPresenter
    * @extends SandboxOutputPresenter
    *
    * @constructor
    *
    * @param {SandboxOutput} sandboxOutput SandboxOutput to be presented.
    */
    var SandboxOutputCbufPresenter = function (sandboxOutput) {
      _.extend(this, sandboxOutput.attributes);
    };

    _.extend(SandboxOutputCbufPresenter.prototype, SandboxOutputPresenter.prototype, {
      /**
      * Labels for Dygraph.
      *
      * @method labels
      * @return {String[]} graph labels
      */
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

      /**
      * Legend labels with IDs for use with Dygraph.
      *
      * @method legendLabels
      * @return {Object[]} legend labels with id and name attributes
      */
      legendLabels: function() {
        var labels = this.labels();

        // Remove "Date"
        labels.shift();

        var i = 0;

        var labelsWithID = _.collect(labels, function(label) {
          return { id: i++, name: label };
        });

        return labelsWithID;
      },

      /**
      * Checks existence of header.
      *
      * @method hasHeader
      * @return {Boolean}
      */
      hasHeader: function() {
        return _.has(this, "header");
      },

      /**
      * Number of seconds per row. Only returns a value if the header is present.
      *
      * @method secondsPerRow
      * @return {Number} seconds
      */
      secondsPerRow: function() {
        if (this.hasHeader()) {
          return this.header.seconds_per_row;
        }
      },

      /**
      * Friendly description of the timespan in seconds, minutes, or hours.
      *
      * @method timespanDescription
      * @return {String} description
      */
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
            numericValue = this._round(seconds / secondsInAMinute);
            timespanDescription = numericValue.toString() + " minute";
          } else {
            numericValue = this._round(seconds / secondsInAnHour);
            timespanDescription = numericValue.toString() + " hour";
          }

          if (numericValue != 1) {
            timespanDescription += "s";
          }

          return timespanDescription;
        }
      },

      /**
      * Rounds numbers to two decimal places if necessary.
      *
      * @method _round
      * @return {Number} rounded number
      * @private
      */
      _round: function(number) {
        return Math.round(number * 100) / 100;
      }
    });

    return SandboxOutputCbufPresenter;
  }
);
