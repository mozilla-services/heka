define(
  [
    "jquery"
  ],
  function($) {
    "use strict";

    /**
    * Circular buffer data container.
    *
    * @class CircularBuffer
    *
    * @constructor
    */
    var CircularBuffer = function() {
      /**
      * Annotations
      *
      * @property {Object} annotations
      */
      this.annotations = null;

      /**
      * Header
      *
      * @property {Object} header
      */
      this.header = null;

      /**
      * Data
      *
      * @property {Object[]} data
      */
      this.data = null;
    };

    /**
    * Parses circular buffer data (files ending in cbuf).
    *
    * @method parse
    *
    * @param {String} input Circular buffer data which includes JSON and CSV data
    *
    * @return {CircularBuffer}
    * @static
    */
    CircularBuffer.parse = function(input) {
      var circularBuffer = new CircularBuffer();
      var lines = input.split("\n");
      var dataStartLine = 1;

      var details = JSON.parse(lines[0]);

      if (details.annotations || details.options) {
        circularBuffer.options = details.options;
        circularBuffer.annotations = details.annotations;
        circularBuffer.header = $.parseJSON(lines[1]);
        dataStartLine = 2;
      } else {
        circularBuffer.header = details;
      }

      circularBuffer.data = [];

      for (var i = dataStartLine; i < lines.length; i++) {
        var line = lines[i];
        var inFields = line.split("\t");
        var fields = [];

        fields[0] = new Date((circularBuffer.header.time + (circularBuffer.header.seconds_per_row * (i - dataStartLine))) * 1000);

        for (var j = 0; j < inFields.length; j++) {
          fields[j + 1] = parseFloat(inFields[j]);
        }

        circularBuffer.data.push(fields);
      }

      return circularBuffer;
    };

    return CircularBuffer;
  }
);
