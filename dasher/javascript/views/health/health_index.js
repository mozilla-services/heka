define(
  [
    "views/base_view",
    "adapters/plugin_adapter",
    "views/health/globals_index",
    "views/health/inputs_index",
    "views/health/decoder_pools_index",
    "views/health/filters_index",
    "views/health/outputs_index",
    "hgn!templates/health/health_index"
  ],
  function(BaseView, PluginAdapter, GlobalsIndex, InputsIndex, DecoderPoolsIndex, FiltersIndex, OutputsIndex, HealthIndexTemplate) {
    "use strict";

    var HealthIndex = BaseView.extend({
      template: HealthIndexTemplate,

      initialize: function() {
        this.pluginAdapter = new PluginAdapter();

        this.globalsIndex = new GlobalsIndex({ collection: this.pluginAdapter.globals });
        this.inputsIndex = new InputsIndex({ collection: this.pluginAdapter.inputs });
        this.decoderPoolsIndex = new DecoderPoolsIndex({ collection: this.pluginAdapter.decoderPools });
        this.filtersIndex = new FiltersIndex({ collection: this.pluginAdapter.filters });
        this.outputsIndex = new OutputsIndex({ collection: this.pluginAdapter.outputs });

        this.pluginAdapter.fill();
      },

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
