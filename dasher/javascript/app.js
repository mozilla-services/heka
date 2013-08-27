App = Ember.Application.create();

App.ApplicationRoute = Ember.Route.extend({
    setupController: function(controller) {
        // `controller` is the instance of ApplicationController
        controller.set('host', "http://127.0.0.1:3000/updates");
    }
});

App.ApplicationController = Ember.Controller.extend({
});

App.Router.map(function() {
    this.route('health');
    this.route('sandboxes');
});

App.IndexRoute = Ember.Route.extend({
});
