define(["underscore", "numeral"], function(_, numeral) {
  "use strict";

  var PluginPresenter = function (plugin) {
    _.extend(this, plugin.attributes);
  };

  _.extend(PluginPresenter.prototype, {
    Name: function() {
      var output = this.Plugin;

      output = output.replace(/DecoderPool-/, "");
      // output = output.replace(/([A-Z])/g, " $1");
      // output = output.replace(/^\s+|\s+$/, "");
      // output = output.replace(/(Input|Decoder|Filter)$/, "")

      return output;
    },

    InChanPercent: function() {
      if (this.InChanLength && this.InChanCapacity) {
        return (this.InChanLength.value / this.InChanCapacity.value) * 100;
      }
    },

    MatchChanPercent: function() {
      if (this.MatchChanLength && this.MatchChanCapacity) {
        return (this.MatchChanLength.value / this.MatchChanCapacity.value) * 100;
      }
    },

    MatchAvgDurationFormatted: function() {
      if (this.MatchAvgDuration) {
        return numeral(this.MatchAvgDuration.value).format("0,0");
      }
    },

    ProcessMessageCountFormatted: function() {
      if (this.ProcessMessageCount) {
        return numeral(this.ProcessMessageCount.value).format("0,0");
      }
    },

    hasMatchChannel: function() {
      return _.has(this, "MatchChanLength");
    }
  });

  return PluginPresenter;
});
