import eslint from '@eslint/js'
import stylistic from '@stylistic/eslint-plugin'
import tseslint from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'

// @ts-ignore
export default tseslint.config(
  // --- Base ---
  eslint.configs.recommended,

  // --- TypeScript: maximum strictness with type-aware rules ---
  // strictTypeChecked = strict + rules requiring type information (no-floating-promises, etc.)
  // stylisticTypeChecked = consistent style enforced via type info
  tseslint.configs.strictTypeChecked,
  tseslint.configs.stylisticTypeChecked,

  // --- Vue: strictest available preset (recommended = essential + strongly-recommended + recommended) ---
  // Using error variant so violations are errors, not warnings.
  pluginVue.configs['flat/recommended-error'],

  // --- Formatting via ESLint Stylistic (replaces Prettier) ---
  stylistic.configs.customize({
    indent: 4,
    quotes: 'single',
    semi: true,
    jsx: false,
    braceStyle: '1tbs',
    arrowParens: true,
    quoteProps: 'consistent-as-needed',
  }),

  // --- TypeScript parser for Vue SFCs ---
  {
    files: ['assets/**/*.vue'],
    languageOptions: {
      globals: {
        document: 'readonly',
        window: 'readonly',
        localStorage: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        HTMLInputElement: 'readonly',
        HTMLSelectElement: 'readonly',
        fetch: 'readonly',
        Response: 'readonly',
        FormData: 'readonly',
        XMLHttpRequest: 'readonly',
        ProgressEvent: 'readonly',
        RequestInit: 'readonly',
      },
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },

  // --- Type-aware linting requires project reference ---
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.vue'],
      },
    },
  },

  // --- Additional strict rules beyond presets ---
  {
    rules: {
      // TypeScript — extra strictness
      // Override stylisticTypeChecked: we prefer `type` over `interface`
      '@typescript-eslint/consistent-type-definitions': ['error', 'type'],
      // Override strictTypeChecked: we require explicit `=== true` / `=== false`
      '@typescript-eslint/no-unnecessary-boolean-literal-compare': 'off',
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-non-null-assertion': 'error',
      '@typescript-eslint/explicit-function-return-type': ['error', {
        allowExpressions: true,
        allowTypedFunctionExpressions: true,
      }],
      '@typescript-eslint/explicit-module-boundary-types': 'error',
      '@typescript-eslint/consistent-type-imports': ['error', {
        prefer: 'type-imports',
        fixStyle: 'inline-type-imports',
      }],
      '@typescript-eslint/consistent-type-exports': ['error', {
        fixMixedExportsWithInlineTypeSpecifier: true,
      }],
      '@typescript-eslint/no-import-type-side-effects': 'error',
      '@typescript-eslint/strict-boolean-expressions': ['error', {
        allowNullableBoolean: true,
      }],
      '@typescript-eslint/switch-exhaustiveness-check': 'error',
      '@typescript-eslint/no-unnecessary-condition': 'error',
      '@typescript-eslint/prefer-readonly': 'error',
      '@typescript-eslint/require-array-sort-compare': 'error',
      '@typescript-eslint/no-misused-promises': ['error', {
        checksVoidReturn: { arguments: false },
      }],

      // General — zero tolerance
      'no-console': 'error',
      'no-debugger': 'error',
      'no-alert': 'error',
      'no-var': 'error',
      'prefer-const': 'error',
      'prefer-template': 'error',
      'object-shorthand': 'error',
      'no-param-reassign': 'error',
      'no-nested-ternary': 'error',
      'no-else-return': 'error',
      'eqeqeq': ['error', 'always'],
      'curly': ['error', 'all'],

      // Stylistic — overrides beyond the preset
      '@stylistic/max-len': ['error', {
        code: 120,
        ignoreUrls: true,
        ignoreStrings: true,
        ignoreTemplateLiterals: true,
        ignoreRegExpLiterals: true,
      }],
      '@stylistic/no-multiple-empty-lines': ['error', { max: 1, maxBOF: 0, maxEOF: 0 }],
      '@stylistic/padding-line-between-statements': ['error',
        { blankLine: 'always', prev: '*', next: 'return' },
        { blankLine: 'always', prev: ['const', 'let'], next: '*' },
        { blankLine: 'any', prev: ['const', 'let'], next: ['const', 'let'] },
        { blankLine: 'always', prev: '*', next: ['interface', 'type'] },
      ],
      '@stylistic/lines-between-class-members': ['error', 'always', {
        exceptAfterSingleLine: true,
      }],

      // Vue — template indent must match script indent (4 spaces)
      'vue/html-indent': ['error', 4],
      'vue/script-indent': ['error', 4, { baseIndent: 0 }],

      // Vue — extra strictness beyond recommended
      'vue/block-lang': ['error', { script: { lang: 'ts' } }],
      'vue/block-order': ['error', { order: ['script', 'template', 'style'] }],
      'vue/component-api-style': ['error', ['script-setup']],
      'vue/component-name-in-template-casing': ['error', 'PascalCase'],
      'vue/custom-event-name-casing': ['error', 'camelCase'],
      'vue/define-emits-declaration': ['error', 'type-based'],
      'vue/define-props-declaration': ['error', 'type-based'],
      'vue/define-macros-order': ['error', {
        order: ['defineProps', 'defineEmits', 'defineSlots'],
        defineExposeLast: true,
      }],
      'vue/html-button-has-type': 'error',
      'vue/no-empty-component-block': 'error',
      'vue/no-ref-object-reactivity-loss': 'error',
      'vue/no-required-prop-with-default': 'error',
      'vue/no-static-inline-styles': 'error',
      'vue/no-useless-mustaches': 'error',
      'vue/no-useless-v-bind': 'error',
      'vue/no-v-text': 'error',
      'vue/padding-line-between-blocks': 'error',
      // Disabled — we intentionally use :class="[...]" arrays for all long class lists
      'vue/prefer-separate-static-class': 'off',
      'vue/prefer-true-attribute-shorthand': 'error',
      'vue/require-macro-variable-name': 'error',
      'vue/require-typed-ref': 'error',
      'vue/v-for-delimiter-style': ['error', 'in'],
    },
  },

  // --- app-ui overrides: allow single-word component names (Button, Input, etc.) ---
  {
    files: ['assets/app-ui/**/*.vue'],
    rules: {
      'vue/multi-word-component-names': 'off',
    },
  },

  // --- Ignored paths ---
  {
    ignores: ['public/**', 'node_modules/**', '*.config.*'],
  },
)
