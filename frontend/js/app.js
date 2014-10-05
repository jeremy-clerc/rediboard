(function(){
	var app = angular.module('rediBoard', ['rediBoardFilters'])
	app.controller('InstancesController', ['$http', function($http) {
		this.sortField = 'name';
		this.errors = [];
		var controller = this;
		controller.instances = []

		this.isSortField = function(sortField) {
			return this.sortField === sortField;
		}
		this.setSortField = function(newField) {
			this.sortField = newField;
		}
		this.hideNoSlave = function(instance) {
			if (instance.errors.length > 0 || instance.connections.length > 0) {
				return true;
			}
			return false;
		}

		$http({method: 'GET', url: '/api/instances'}).
		success(function(data, status, headers, config) {
			controller.instances = data.instances;
			controller.errors = controller.errors.concat(data.errors);
		}).
		error(function(data, status, headers, config) {
			if (status == 502) {
				statusText = "It seems the API is not running (Error: 502)";
			} else {
				statusText = "HTTP Error code " + status;
			}
			controller.errors.push("Error getting instances list. " + statusText)
		});
	}]);
})();
