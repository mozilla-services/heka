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
    "raphael":               "vendor/raphael",
    "justgage":              "vendor/justgage",
    "jquery.animateNumbers": "vendor/jquery.animateNumbers"
  },

  shim: {
    "underscore": {
      exports: "_"
    },

    "backbone": {
      deps: ["jquery", "underscore"],
      exports: "Backbone"
    },

    "raphael": {
      exports: "Raphael"
    },

    "justgage": {
      deps: ["raphael"],
      exports: "JustGage"
    },

    "jquery.animateNumbers": {
      deps: ["jquery"],
      exports: "jQuery.fn.animateNumbers"
    }
  }
});

define(["backbone", "router"], function (Backbone, Router) {
  "use strict";

  var router = new Router();

  Backbone.history.start();
});
