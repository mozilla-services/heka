define(
  [
    "underscore",
    "crc32"
  ],
  function(_, crc32) {
    "use strict";

    /**
    * Termination report data container.
    *
    * @class TerminationReport
    *
    * @constructor
    */
    var TerminationReport = function() {
      /**
      * Data
      *
      * @property {Object[]} data
      */
      this.data = [];
    };

    /**
    * Parses termination report from tsv.
    *
    * @method parse
    *
    * @param {String} input Tab delimited termination report data
    *
    * @return {TerminationReport}
    * @static
    */
    TerminationReport.parse = function(input) {
      var terminatationReport = new TerminationReport();
      var lines = input.split("\n");

      _.each(lines, function(line) {
        if (line.length > 0) {
          var fields = line.split("\t");

          terminatationReport.data.push({
            id: crc32(fields[0] + fields[1] + fields[2]),
            time: new Date(parseInt(fields[0], 10) * 1000),
            plugin: fields[1],
            message: fields[2]
          });
        }
      });

      return terminatationReport;
    };

    return TerminationReport;
  }
);
