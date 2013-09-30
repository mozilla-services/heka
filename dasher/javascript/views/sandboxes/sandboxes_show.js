define(
  [
    "views/base_view",
    "hgn!templates/sandboxes/sandboxes_show"
  ],
  function(BaseView, SandboxesShowTemplate) {
    "use strict";

    var SandboxesShow = BaseView.extend({
      template: SandboxesShowTemplate,

      initialize: function() {

      }
    });

    return SandboxesShow;
  }
);
