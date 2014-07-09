define(
  [
    "views/base_view",
    "adapters/plugins_adapter",
    "views/health/globals_index",
    "views/health/inputs_index",
    "views/health/decoders_index",
    "views/health/filters_index",
    "views/health/outputs_index",
    "views/health/encoders_index",
    "hgn!templates/health/health_index"
  ],
  function(BaseView, PluginsAdapter, GlobalsIndex, InputsIndex, DecodersIndex,
    FiltersIndex, OutputsIndex, EncodersIndex, HealthIndexTemplate) {
    "use strict";

    /**
    * Index view for health views. This is a top level view that's loaded by the router.
    *
    * @class HealthIndex
    * @extends BaseView
    *
    * @constructor
    */
    var HealthIndex = BaseView.extend({
      template: HealthIndexTemplate,

      initialize: function() {
        this.adapter = PluginsAdapter.instance();

        this.globalsIndex = new GlobalsIndex({ collection: this.adapter.globals });
        this.inputsIndex = new InputsIndex({ collection: this.adapter.inputs });
        this.decodersIndex = new DecodersIndex({ collection: this.adapter.decoders });
        this.filtersIndex = new FiltersIndex({ collection: this.adapter.filters });
        this.outputsIndex = new OutputsIndex({ collection: this.adapter.outputs });
        this.encodersIndex = new EncodersIndex({ collection: this.adapter.encoders });

        this.adapter.fill();
      },

      /**
      * Renders globalsIndex, inputsIndex, decodersIndex, filtersIndex,
      * outputsIndex, encodersIndex into the corresponding DOM elements.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.assign(this.globalsIndex, "#globals-index");
        this.assign(this.inputsIndex, "#inputs-index");
        this.assign(this.decodersIndex, "#decoders-index");
        this.assign(this.filtersIndex, "#filters-index");
        this.assign(this.outputsIndex, "#outputs-index");
        this.assign(this.encodersIndex, "#encoders-index");
      }
    });

    return HealthIndex;
  }
);
