define(
  [
    "underscore",
    "jquery",
    "crc32"
  ],
  function(_, $, crc32) {
    "use strict";

    /**
    * Base adapter handles fetching data via XHR and polling for changes.
    *
    * @class BaseAdapter
    * @constructor
    */
    var BaseAdapter = function() {};

    _.extend(BaseAdapter.prototype, {
      /**
      * Fills local variables with data fetched from the server. Should be implemented by
      * subclasses.
      *
      * @method fill
      */
      fill: function() {
        throw "fill not implemented.";
      },

      /**
      * Fetches data from the server using XHR. Only calls back if the data has changed.
      *
      * @method fetch
      * @param {String} url URL of the resource to be fetched
      * @param {Function} callback Function called when new data is received from the server
      * @param {Object} callback.data Data returned from the request
      * @param {String} callback.textStatus Status returned from the request
      * @param {jqXHR} callback.jqXHR jQuery XHR object representing the request
      */
      fetch: function(url, callback) {
        $.ajax(url, { cache: false }).then(function(data, textStatus, jqXHR) {
          var responseTextCode = crc32(jqXHR.responseText);

          if (this.lastFetchResponseCode !== responseTextCode) {
            this.lastFetchResponseCode = responseTextCode;

            callback(data, textStatus, jqXHR);
          }
        }.bind(this));
      },

      /**
      * Calls `fill` every 2 seconds to check for new data. Only allows one timer to be running.
      *
      * @method pollForUpdates
      */
      pollForUpdates: function() {
        if (!this.updateTimer) {
          this.updateTimer = setInterval(function() { this.fill(); }.bind(this), 2000);
        }
      },

      /**
      * Stops calling `fill`.
      *
      * @method stopPollingForUpdates
      */
      stopPollingForUpdates: function() {
        clearTimeout(this.updateTimer);
        delete this.updateTimer;
      }
    });

    return BaseAdapter;
  }
);
