define(["underscore", "numeral", "presenters/plugin_presenter"], function(_, numeral, PluginPresenter) {
  "use strict";

  var DecoderPoolPluginPresenter = function (plugin) {
    _.extend(this, plugin.attributes);

    this.decoders = plugin.get("decoders").collect(function(decoder) {
      return new PluginPresenter(decoder);
    });
  };

  _.extend(DecoderPoolPluginPresenter.prototype, PluginPresenter.prototype, {
    Name: function() {
      return this.Plugin.replace(/DecoderPool-/, "");
    },
  });

  return DecoderPoolPluginPresenter;
});
