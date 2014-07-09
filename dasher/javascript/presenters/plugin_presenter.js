define(
  [
    "underscore",
    "numeral",
    "presenters/sandbox_output_presenter"
  ],
  function(_, numeral, SandboxOutputPresenter) {
    "use strict";

    /**
    * Presents a Plugin for use in a view.
    *
    * @class PluginPresenter
    * @extends Plugin
    *
    * @constructor
    *
    * @param {Plugin} plugin Plugin to be presented
    */
    var PluginPresenter = function (plugin) {
      this.plugin = plugin;

      // Used for nested contexts with name collisions
      this.PluginName = plugin.get("Name");

      // Copy plugin attributes
      _.extend(this, plugin.attributes);

      // Load presenters for each output
      if (this.Outputs) {
        this.Outputs = this.Outputs.collect(function(sandboxOutput) {
          return new SandboxOutputPresenter(sandboxOutput);
        });
      }
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
      * Process message average duration formatted with commas.
      *
      * @method ProcessMessageAvgDurationFormatted
      * @return {String} comma delimited number
      */
      ProcessMessageAvgDurationFormatted: function() {
        if (this.ProcessMessageAvgDuration) {
          return numeral(this.ProcessMessageAvgDuration.value).format("0,0");
        }
      },

      /**
      * Process message failures formatted with commas.
      *
      * @method ProcessMessageFailuresFormatted
      * @return {String} comma delimited number
      */
      ProcessMessageFailuresFormatted: function() {
        if (this.ProcessMessageFailures) {
          return numeral(this.ProcessMessageFailures.value).format("0,0");
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
      * Glyphicon CSS class based on the plugin type.
      *
      * @method typeCSSClass
      * @return {String} cssClass
      */
      typeCSSClass: function() {
        var cssClass = "glyphicon-";

        switch (this.Type) {
          case "Input":
            cssClass += "log-in";
            break;
          case "Decoder":
            cssClass += "import";
            break;
          case "Filter":
            cssClass += "filter";
            break;
          case "Output":
            cssClass += "log-out";
            break;
          case "Encoder":
            cssClass += "export";
            break;
          default:
            cssClass = "no-icon";
        }

        return cssClass;
      },

      /**
      * Checks existence of a match channel.
      *
      * @method hasMatchChannel
      * @return {Boolean}
      */
      hasMatchChannel: function() {
        return _.has(this, "MatchChanLength");
      },

      /**
      * Checks existence of outputs.
      *
      * @method hasOutputs
      * @return {Boolean}
      */
      hasOutputs: function() {
        return _.has(this, "Outputs");
      },

      /**
      * Checks existence of keys and values.
      *
      * @method hasKeysAndValues
      * @return {Boolean}
      */
      hasKeysAndValues: function() {
        return this.keysAndValues().length > 0;
      },

      /**
      * Array of properties and formatted keys and values.
      *
      * @method keysAndValues
      * @return {Object[]} keysAndValues
      */
      keysAndValues: function() {
        var keysAndValues = [];

        _.each(this.plugin.attributes, function(value, key) {
          if (key != "Name" && key != "id" && key != "Outputs" && key != "Type") {
            var formattedValue;
            if (value.representation != "") {
              formattedValue = numeral(value.value).format("0,0");
            } else {
              formattedValue = value.value;
            }

            if (value.representation && value.representation != "count") {
              formattedValue += " " + value.representation;
            }

            keysAndValues.push({ key: key, value: formattedValue });
          }
        });

        return keysAndValues;
      }
    });

    return PluginPresenter;
  }
);
