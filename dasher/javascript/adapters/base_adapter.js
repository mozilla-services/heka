define(
  [
    "underscore",
    "backbone",
    "jquery",
    "crc32"
  ],
  function(_, Backbone, $, crc32) {
    "use strict";

    var BaseAdapter = function() {
      // do nothing
    };

    _.extend(BaseAdapter.prototype, {
      fetch: function(url, callback) {
        $.ajax(url, { cache: false }).then(function(data, textStatus, jqXHR) {
          var responseTextCode = crc32(jqXHR.responseText);

          if (this.lastFetchResponseCode !== responseTextCode) {
            this.lastFetchResponseCode = responseTextCode;

            callback(data, textStatus, jqXHR);
          }
        }.bind(this));
      },

      listenForUpdates: function() {
        if (!this.updateTimer) {
          this.updateTimer = setInterval(function() { this.fill(); }.bind(this), 2000);
        }
      },

      stopListeningForUpdates: function() {
        clearTimeout(this.updateTimer);
        delete this.updateTimer;
      }
    });

    return BaseAdapter;
  }
);
