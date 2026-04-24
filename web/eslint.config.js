// @ts-check
const { FlatCompat } = require('@eslint/eslintrc');
const { defineConfig } = require('eslint/config');

const compat = new FlatCompat({
  baseDirectory: __dirname,
});

module.exports = defineConfig([
  {
    ignores: ['dist/**', 'coverage/**', 'projects/**/*'],
  },
  ...compat.config({
    overrides: [
      {
        files: ['src/**/*.ts'],
        extends: [
          'plugin:@angular-eslint/recommended',
          'plugin:@angular-eslint/template/process-inline-templates',
        ],
        rules: {},
      },
      {
        files: ['src/**/*.html'],
        extends: ['plugin:@angular-eslint/template/recommended'],
        rules: {},
      },
    ],
  }),
]);
