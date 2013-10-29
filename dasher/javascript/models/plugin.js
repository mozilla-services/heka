define(
  [
    "backbone",
    "deepModel"
  ],
  function(Backbone) {
    "use strict";

    /**
    * Plugin model representing any of the four plugin types: inputs, decoders, filters, or outputs.
    *
    * @class Plugin
    *
    * @constructor
    */
    var Plugin = Backbone.DeepModel.extend({});

    /**
    * Unique identifier for the plugin (same as `Name`)
    *
    * @property {String} id
    */

    /**
    * Name of the plugin.
    *
    * @property {String} Name
    */

    /**
    * Number of messages processed.
    *
    * @property {Object} ProcessMessageCount
    * @property {Number} ProcessMessageCount.value
    * @property {String} ProcessMessageCount.representation
    */

    /**
    * Number of messages injected.
    *
    * @property {Object} InjectMessageCount
    * @property {Number} InjectMessageCount.value
    * @property {String} InjectMessageCount.representation
    */

    /**
    * Capacity of the in channel.
    *
    * @property {Object} InChanCapacity
    * @property {Number} InChanCapacity.value
    * @property {String} InChanCapacity.representation
    */

    /**
    * Number of messages in the in channel.
    *
    * @property {Object} InChanLength
    * @property {Number} InChanLength.value
    * @property {String} InChanLength.representation
    */

    /**
    * Capacity of the match channel.
    *
    * @property {Object} MatchChanCapacity
    * @property {Number} MatchChanCapacity.value
    * @property {String} MatchChanCapacity.representation
    */

    /**
    * Number of messages in the match channel.
    *
    * @property {Object} MatchChanLength
    * @property {Number} MatchChanLength.value
    * @property {String} MatchChanLength.representation
    */

    /**
    * Average match duration.
    *
    * @property {Object} MatchAvgDuration
    * @property {Number} MatchAvgDuration.value
    * @property {String} MatchAvgDuration.representation
    */

    return Plugin;
  }
);
