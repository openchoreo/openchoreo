module.exports = require('@backstage/cli/config/eslint-factory')(__dirname, {
  ignorePatterns: ['templates/'],
  rules: {
    'react/react-in-jsx-scope': 'off',
  },
});
