/* globals app */

app.directive('checkbox', function() {
  return {
    restrict: 'E',
    replace: true,
    transclude: true,
    scope: {
      type: "@",
      size: "@",
      checked: "@",
      disabled: "@",
      model: '=ngModel'
    },
    template: "<div class=\"{{checkbox_class}}\">" +
      "<input type=\"checkbox\">" +
      "<label ng-click=\"click_on_checkbox()\" ng-transclude></label>" +
      "</div>",
    link: function(scope, element, attrs, ngModel) {
      // set up checkbox class and type
      if (scope.type == 'standard' || scope.type === undefined) {
        scope.type = 'standard';
        scope.checkbox_class = 'ui checkbox';
      } else if (scope.type == 'slider') {
        scope.type = 'slider';
        scope.checkbox_class = 'ui slider checkbox';
      } else if (scope.type == 'toggle') {
        scope.type = 'toggle';
        scope.checkbox_class = 'ui toggle checkbox';
      } else {
        scope.type = 'standard';
        scope.checkbox_class = 'ui checkbox';
      }
      // set checkbox size
      if (scope.size == 'large') {
        scope.checkbox_class = scope.checkbox_class + ' large';
      } else if (scope.size == 'huge') {
        scope.checkbox_class = scope.checkbox_class + ' huge';
      }
      // set checked/unchecked
      if (scope.checked == 'false' || scope.checked === undefined) {
        scope.checked = false;
      } else {
        scope.checked = true;
        element.children()[0].setAttribute('checked', '');
      }
      // check if the parameter disabled is available
      if (scope.disabled == 'disabled') {
        scope.checkbox_class += ' disabled';
      }
      // Click handler
      element.bind('click', function() {
        scope.$apply(function() {
          if (scope.disabled === undefined) {
            if (scope.checked === true) {
              scope.checked = true;
              scope.model = false;
              element.children()[0].removeAttribute('checked');
            } else {
              scope.checked = true;
              scope.model = true;
              element.children()[0].setAttribute('checked', 'true');
            }
          }
        });
      });
      // Watch for ng-model
      scope.$watch('model', function(val) {
        if (val === undefined)
          return;

        if (val === true) {
          scope.checked = true;
          element.children()[0].setAttribute('checked', 'true');
        } else {
          scope.checked = false;
          element.children()[0].removeAttribute('checked');
        }
      });
    }
  };
});
