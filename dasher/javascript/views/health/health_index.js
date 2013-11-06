define(
  [
    "views/base_view",
    "adapters/plugins_adapter",
    "views/health/globals_index",
    "views/health/inputs_index",
    "views/health/decoder_pools_index",
    "views/health/filters_index",
    "views/health/outputs_index",
    "hgn!templates/health/health_index"
  ],
  function(BaseView, PluginsAdapter, GlobalsIndex, InputsIndex, DecoderPoolsIndex, FiltersIndex, OutputsIndex, HealthIndexTemplate) {
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
        this.decoderPoolsIndex = new DecoderPoolsIndex({ collection: this.adapter.decoderPools });
        this.filtersIndex = new FiltersIndex({ collection: this.adapter.filters });
        this.outputsIndex = new OutputsIndex({ collection: this.adapter.outputs });

        this.adapter.fill();
      },

      /**
      * Renders globalsIndex, inputsIndex, decoderPoolsIndex, filtersIndex, outputsIndex into the
      * corresponding DOM elements.
      *
      * @method afterRender
      */
      afterRender: function() {
        this.assign(this.globalsIndex, "#globals-index");
        this.assign(this.inputsIndex, "#inputs-index");
        this.assign(this.decoderPoolsIndex, "#decoder-pools-index");
        this.assign(this.filtersIndex, "#filters-index");
        this.assign(this.outputsIndex, "#outputs-index");
      }
    });

    return HealthIndex;
  }
);
