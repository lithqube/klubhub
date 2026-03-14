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
      rules: {},
    },
    {
      ignores: ['.nuxt/**', '.output/**', 'node_modules'],
    },
  );
