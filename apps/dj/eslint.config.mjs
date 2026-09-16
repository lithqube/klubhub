import { createConfigForNuxt } from '@nuxt/eslint-config/flat';
import baseConfig from '../../eslint.config.mjs';

export default createConfigForNuxt({
  features: {
    typescript: true,
  },
})
  .prepend(...baseConfig)
  .append(
    {
      files: ['**/*.ts', '**/*.tsx', '**/*.js', '**/*.jsx', '**/*.vue'],
      rules: {
        // v1 baseline: legacy mock handlers and stores still contain explicit
        // `any`. Keep the debt visible without making the first CI lint gate
        // impossible to adopt; new code should use concrete types.
        '@typescript-eslint/no-explicit-any': 'warn',
      },
    },
    {
      ignores: ['.nuxt/**', '.output/**', 'node_modules'],
    },
  );
