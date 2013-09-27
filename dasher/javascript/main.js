require.config({
  enforceDefine: true,
  baseUrl: "javascript",

  paths: {
    "jquery":                "vendor/jquery",
    "underscore":            "vendor/underscore",
    "backbone":              "vendor/backbone",
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

    "jquery.animateNumbers": {
      deps: ["jquery"],
      exports: "jQuery.fn.animateNumbers"
    },

    "dygraph": {
      exports: "Dygraph"
    }
  }
});

define(["backbone", "router"], function (Backbone, Router) {
  "use strict";

  var router = new Router();

  Backbone.history.start();
});
