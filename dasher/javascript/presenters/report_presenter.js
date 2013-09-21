define(["underscore", "numeral"], function(_, numeral) {
  "use strict";

  var ReportPresenter = function (report) {
    _.extend(this, report.attributes);
  };

  _.extend(ReportPresenter.prototype, {
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

  return ReportPresenter;
});
