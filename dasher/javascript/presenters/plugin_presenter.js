define(
  [
    "underscore",
    "numeral"
  ],
  function(_, numeral) {
    "use strict";

    /**
    * Presents a Plugin for use in a view.
    *
    * @class PluginPresenter
    * @extends Plugin
    *
    * @constructor
    *
    * @param {Plugin} plugin Plugin to be presented.
    */
    var PluginPresenter = function (plugin) {
      _.extend(this, plugin.attributes);
    };

    _.extend(PluginPresenter.prototype, {
      /**
      * In channel percent filled.
      *
      * @method InChanPercent
      * @return {Number} between 0 and 100
      */
      InChanPercent: function() {
        if (this.InChanLength && this.InChanCapacity) {
          return (this.InChanLength.value / this.InChanCapacity.value) * 100;
        }
      },

      /**
      * Match channel percent filled.
      *
      * @method MatchChanPercent
      * @return {Number} between 0 and 100
      */
      MatchChanPercent: function() {
        if (this.MatchChanLength && this.MatchChanCapacity) {
          return (this.MatchChanLength.value / this.MatchChanCapacity.value) * 100;
        }
      },

      /**
      * Match channel average duration formatted with commas.
      *
      * @method MatchAvgDurationFormatted
      * @return {String} comma delimited number
      */
      MatchAvgDurationFormatted: function() {
        if (this.MatchAvgDuration) {
          return numeral(this.MatchAvgDuration.value).format("0,0");
        }
      },

      /**
      * Processed message count formatted with commas.
      *
      * @method ProcessMessageCountFormatted
      * @return {String} comma delimited number
      */
      ProcessMessageCountFormatted: function() {
        if (this.ProcessMessageCount) {
          return numeral(this.ProcessMessageCount.value).format("0,0");
        }
      },

      /**
      * Description of messages in channel.
      *
      * @method ChanDescription
      * @return {String} description
      */
      ChanDescription: function() {
        if (this.Name === "inputRecycleChan" || this.Name === "injectRecycleChan") {
          return "Messages Available";
        } else {
          return "Messages in Channel";
        }
      },

      /**
      * Checks existence of a match channel
      *
      * @method hasMatchChannel
      * @return {Boolean}
      */
      hasMatchChannel: function() {
        return _.has(this, "MatchChanLength");
      }
    });

    return PluginPresenter;
  }
);
