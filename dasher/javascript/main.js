// Require.js configuration. Provides short names for vendor libs and shims for non AMD libs.
require.config({
  enforceDefine: true,
  baseUrl: "javascript",

  paths: {
    "jquery":                "vendor/jquery",
    "underscore":            "vendor/underscore",
    "backbone":              "vendor/backbone",
    "bootstrap":             "vendor/bootstrap.min",
    "text":                  "vendor/text",
    "hgn":                   "vendor/hgn",
    "hogan":                 "vendor/hogan",
    "numeral":               "vendor/numeral",
    "dygraph":               "vendor/dygraph-combined",
    "deepModel":             "vendor/deep-model",
    "crc32":                 "vendor/crc32",
    "moment":                "vendor/moment.min"
  },

  shim: {
    "underscore": {
      exports: "_"
    },

    "backbone": {
      deps: ["jquery", "underscore"],
      exports: "Backbone"
    },

    "bootstrap": {
      deps: ["jquery"],
      // this is misleading, but we have to export something and bootstrap contains many plugins
      exports: "$.fn.alert"
    },

    "dygraph": {
      exports: "Dygraph"
    },

    "crc32": {
      exports: "crc32"
    }
  }
});

// Sets up the router and starts the history which inserts the appropriate Backbone view into #content.
define(
  [
    "backbone",
    "router",
    "bootstrap"
  ],
  function (Backbone, Router) {
    "use strict";

    var router = new Router();

    Backbone.history.start();
  }
);
