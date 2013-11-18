define(
  [
    "underscore",
    "presenters/plugin_presenter"
  ],
  function(_, PluginPresenter) {
    "use strict";

    /**
    * Presents a decoder pool Plugin for use in a view.
    *
    * @class DecoderPoolPluginPresenter
    * @extends PluginPresenter
    *
    * @constructor
    *
    * @param {Plugin} plugin Plugin to be presented
    */
    var DecoderPoolPluginPresenter = function (plugin) {
      _.extend(this, plugin.attributes);

      this.decoders = plugin.get("decoders").collect(function(decoder) {
        return new PluginPresenter(decoder);
      });
    };

    _.extend(DecoderPoolPluginPresenter.prototype, PluginPresenter.prototype);

    return DecoderPoolPluginPresenter;
  }
);
