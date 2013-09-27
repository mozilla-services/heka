define(["views/base_view", "hgn!templates/sandboxes/sandboxes_index"], function(BaseView, SandboxesIndexTemplate) {
  "use strict";

  var SandboxesIndex = BaseView.extend({
    template: SandboxesIndexTemplate,

    initialize: function() {
    }
  });

  return SandboxesIndex;
});
