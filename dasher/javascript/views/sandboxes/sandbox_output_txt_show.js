define(
  [
    "views/base_view",
    "hgn!templates/sandboxes/sandbox_output_txt_show",
    "adapters/sandbox_output_txt_adapter",
    "presenters/sandbox_output_presenter"
  ],
  function(BaseView, SandboxOutputTxtShowTemplate, SandboxOutputTxtAdapter, SandboxOutputPresenter) {
    "use strict";

    /**
    * Show view for text based sandbox outputs.
    *
    * @class SandboxOutputTxtShow
    * @extends BaseView
    *
    * @constructor
    */
    var SandboxOutputTxtShow = BaseView.extend({
      template: SandboxOutputTxtShowTemplate,
      presenter: SandboxOutputPresenter,
      className: "sandboxes-output sandboxes-output-txt",

      initialize: function() {
        this.adapter = new SandboxOutputTxtAdapter(this.model);

        this.listenTo(this.model, "change:Name", this.updateName, this);
        this.listenTo(this.model, "change:data", this.updateData, this);

        this.adapter.fill();
      },

      /**
      * Efficiently updates the name.
      *
      * @method updateName
      */
      updateName: function() {
        this.$("h4").html(this.model.get("Name"));
      },

      /**
      * Efficiently updates the preformatted data.
      *
      * @method updateData
      */
      updateData: function() {
        this.$("pre").html(this.model.get("data"));
      },

      /**
      * Stops polling for updates before being destroyed.
      *
      * @method beforeDestroy
      */
      beforeDestroy: function() {
        this.adapter.stopPollingForUpdates();
      }
    });

    return SandboxOutputTxtShow;
  }
);
