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
      * Finds an element in a collection asynchronously.
      *
      * @method findWhere
      *
      * @param {Backbone.Collection} collection Collection to be searched
      * @param {Object} options Search options that are passed to `Backbone.Collection.findWhere`
      * @param {Function} callback Function called when find is complete
      * @param {String} callback.result Model found from the search.
      */
      findWhere: function(collection, options, callback) {
        if (collection.length > 0) {
          callback(collection.findWhere(options));
        } else {
          this.fill().done(function() {
            callback(collection.findWhere(options));
          });
        }
      },
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
        $.ajax(url, { dataType: this.getDataType(url), cache: false }).then(function(data, textStatus, jqXHR) {
          var responseTextCode = crc32(jqXHR.responseText);

          if (this.lastFetchResponseCode !== responseTextCode) {
            this.lastFetchResponseCode = responseTextCode;

            callback(data, textStatus, jqXHR);
          }
        }.bind(this));
      },

      /**
      * Determines the jQuery ajax data type based the extension in a URL.
      *
      * This makes the dashboard more resilient to incorrect mime types returned from the server.
      *
      * @method getDataType
      * @param {String} url URL used to determine the data type
      */
      getDataType: function(url) {
        var dataType;

        if (url.match(/\.json$/)) {
          dataType = "json";
        } else if (url.match(/\.xml$/)) {
          dataType = "xml";
        } else {
          dataType = "text";
        }

        return dataType;
      },

      /**
      * Calls `fill` every 2 seconds to check for new data. Only allows one timer to be running.
      *
      * @method pollForUpdates
      */
      pollForUpdates: function(ms) {
        // default to 2 seconds
        ms = ms || 2000;

        if (!this.updateTimer) {
          this.updateTimer = setInterval(function() { this.fill(); }.bind(this), ms);
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
