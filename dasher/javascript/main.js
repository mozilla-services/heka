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
    "jquery.animateNumbers": "vendor/jquery.animateNumbers",
    "dygraph":               "vendor/dygraph-combined"
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

    "jquery.animateNumbers": {
      deps: ["jquery"],
      exports: "jQuery.fn.animateNumbers"
    },

    "dygraph": {
      exports: "Dygraph"
    }
  }
});

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
