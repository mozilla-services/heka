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
      * Friendly description of the aggregation units in seconds, 
      * minutes, hours, or days. 
      *
      * @method aggregationUnits
      * @return {String} description
      */
      aggregationUnits: function(seconds, plural) {
          var secondsInAMinute = 60;
          var secondsInAnHour = 3600;
          var secondsInADay = secondsInAnHour * 24;
          var description;
          var numericValue;

          if (seconds < secondsInAMinute) {
            numericValue = seconds;
            description = numericValue.toString() + " second";
          } else if (seconds < secondsInAnHour) {
            numericValue = this._round(seconds / secondsInAMinute);
            description = numericValue.toString() + " minute";
          } else if (seconds < secondsInADay) {
            numericValue = this._round(seconds / secondsInAnHour);
            description = numericValue.toString() + " hour";
          } else {
            numericValue = this._round(seconds / secondsInADay);
            description = numericValue.toString() + " day";
          }
          
          if (plural && numericValue != 1) {
            description += "s";
          }
          if (plural && numericValue == 1) {
              description = description.substr(2)
          }

          return description;
      },

      /**
      * Friendly description of the circular buffer aggregation 
      * window. 
      *
      * @method aggregationDescription
      * @return {String} description
      */
      aggregationDescription: function() {
        if (this.hasHeader()) {
          var aggregate = this.aggregationUnits(this.header.seconds_per_row, false);
          var span = this.aggregationUnits(this.header.seconds_per_row * this.header.rows, true);
          return aggregate + " aggregation for the last " + span;
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
