define(["jquery"], function($) {
  "use strict";

  var CircularBufferParser = {
    parse: function(data) {
      var circularBuffer = {};
      var lines = data.split("\n");
      var start = 1;

      var obj = JSON.parse(lines[0]);

      if (obj.annotations) {
        circularBuffer.annotations = obj.annotations;
        circularBuffer.header = $.parseJSON(lines[1]);
        start = 2;
      } else {
        circularBuffer.header = obj;
      }

      circularBuffer.data = [];

      for (var i = start; i < lines.length; i++) {
        var line = lines[i];
        var inFields = line.split("\t");
        var fields = [];

        fields[0] = new Date((circularBuffer.header.time + (circularBuffer.header.seconds_per_row * (i - start))) * 1000);

        for (var j = 0; j < inFields.length; j++) {
          fields[j + 1] = parseFloat(inFields[j]);
        }

        circularBuffer.data.push(fields);
      }

      return circularBuffer;
    }
  };

  return CircularBufferParser;
});
