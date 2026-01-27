export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'scope-enum': [
      2,
      'always',
      [
        'api',
        'controller',
        'cli',
        'helm',
        'deps',
      ],
    ],
    'scope-empty': [0], // scope is optional
    'body-max-line-length': [0], // disable body line length limit
  },
};
