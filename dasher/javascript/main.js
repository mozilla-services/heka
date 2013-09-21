require.config({
  enforceDefine: true,
  baseUrl: "javascript",

  paths: {
    jquery:     "vendor/jquery",
    underscore: "vendor/underscore",
    backbone:   "vendor/backbone",
    text:       "vendor/text",
    hgn:        "vendor/hgn",
    hogan:      "vendor/hogan",
    numeral:    "vendor/numeral",
    raphael:    "vendor/raphael",
    justgage:   "vendor/justgage"
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
    }
  }
});

define(["backbone", "router"], function (Backbone, Router) {
  "use strict";

  var router = new Router();

  Backbone.history.start();
});
