(function(){
	angular.module('rediBoardFilters', []).filter('human', function() {
		return function(input) {
			if (input == 0) {
				return "unlimited";
			} else if (input < 1024) {
				return input + " B";
			} else if (input < 1048576) {
				return (input / 1024) + " KB";
			} else if (input < 1073741824) {
				return (input / 1024 / 1024) + " MB";
			} else if (input != 0) {
				return (input / 1024 / 1024 / 1024) + " GB";
			}
			return input;
		};
	});
})();
