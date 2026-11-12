// @ts-check
const { defineConfig } = require('eslint/config');
const angular = require('angular-eslint');

module.exports = defineConfig([
  {
    ignores: ['dist/**', 'coverage/**', 'projects/**/*'],
  },
  {
    files: ['src/**/*.ts'],
    extends: [angular.configs.tsRecommended],
    processor: angular.processInlineTemplates,
    rules: {
      // Angular 22 defaults to OnPush; the upgrade migration pins existing components to Eager to
      // keep v21 behaviour. Drop this override once components are moved to OnPush.
      '@angular-eslint/prefer-on-push-component-change-detection': 'off',
    },
  },
  {
    files: ['src/**/*.html'],
    extends: [angular.configs.templateRecommended],
    rules: {},
  },
]);
