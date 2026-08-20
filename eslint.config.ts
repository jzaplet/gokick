import eslint from '@eslint/js'
import stylistic from '@stylistic/eslint-plugin'
import tseslint from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import sonarjs from 'eslint-plugin-sonarjs'

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

      // --- Size / depth gates (audit ratchet — enabled as preconditions clear) ---
      // Measured green 2026-07-13: largest file 193 lines, max nesting depth 3.
      // The rest of the ratchet landed separately: no-as and cognitive
      // complexity have their own blocks below. Cyclomatic complexity is NOT
      // coming — see the cognitive-complexity block for why.
      'max-lines': ['error', { max: 300, skipBlankLines: true, skipComments: true }],
      'max-depth': ['error', 4],
      // NOT max-lines-per-function, though Go gates function length via funlen.
      // It is not the same measurement here: Go's "methods" are separate
      // top-level funcs that funlen bills one by one, while a JS factory's
      // methods are closures INSIDE it, so the rule bills all of them to the
      // parent. createGridState is 149 lines of ~15 small closures at cognitive
      // complexity 11 — followable, and splitting it would only scatter the
      // state it exists to hold. File size (max-lines) plus cognitive
      // complexity cover this shape; a line count per function does not.

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

  // --- Wire-boundary discipline (typová parita ③; the FE half of `gk boundary`) ---
  // TBody = never already makes an UNDECLARED body a compile error; these rules
  // close the inference loophole (zero type args => TS infers TData=unknown and
  // TBody from the literal) and keep payloads flowing from typed variables, not
  // ad-hoc literals. Scoped to assets/ — transport tests in tests/ exercise the
  // mechanics with ad-hoc bodies on purpose.
  // --- Ratchet: no type assertions ---
  // `as` is the one construct that lets a value LIE about its type with zero
  // diagnostics — the opposite of everything the parity loop enforces. The
  // runtime-guard boundary cleared the inventory down to two irreducible casts
  // (parseResponse's caller-supplied generics, where there is no guard to
  // call); both carry an eslint-disable naming the reason, the same way the
  // Go raw-pool exceptions do. `as const` is NOT an assertion in this rule's
  // eyes (verified) — the Role/Permission enums keep working untouched.
  {
    files: ['assets/**/*.{ts,vue}'],
    rules: {
      '@typescript-eslint/consistent-type-assertions': ['error', { assertionStyle: 'never' }],
    },
  },

  // --- Cognitive complexity (Campbell's metric — the Go half is gocognit) ---
  // Measures how hard code is to FOLLOW, not how many paths it has: +1 per
  // break in linear flow, plus a penalty for nesting. Deliberately NOT
  // cyclomatic (ESLint's `complexity`, Go's `cyclop`) — that one bills a flat
  // validation chain the same as deep nesting and pushes you to hide branches
  // in helpers rather than simplify them.
  // 15 is the metric's default and the ceiling the code already meets
  // (parseResponse sits exactly at it) — a ratchet, so no slack by design.
  {
    files: ['assets/**/*.{ts,vue}'],
    plugins: { sonarjs },
    rules: {
      'sonarjs/cognitive-complexity': ['error', 15],
    },
  },

  {
    files: ['assets/**/*.{ts,vue}'],
    rules: {
      'no-restricted-syntax': ['error',
        {
          selector: 'CallExpression[callee.name=/^(authFetch|apiFetch|apiUpload)$/]:not([typeArguments])',
          message: 'Declare explicit generics — authFetch<Res, Err[, Req]> / apiUpload<Res, Err> — with the tsgen-generated types; inference lets an untyped payload through.',
        },
        {
          selector: 'CallExpression[callee.name=/^(authFetch|apiFetch)$/] > ObjectExpression.arguments > Property[key.name=\'body\'] > :matches(ObjectExpression, ArrayExpression).value',
          message: 'No inline body literals — pass a variable typed with the tsgen-generated request type (e.g. UserFormData), so the payload shape is pinned to the Go DTO.',
        },
      ],
    },
  },

  // apiFetchCore is the LOOSE implementation (validate optional, TBody
  // inferrable) — importable only by the two facades that re-add the
  // contract. Anywhere else it would bypass the whole validate↔TData
  // discipline with zero diagnostics.
  {
    files: ['assets/**/*.{ts,vue}'],
    ignores: ['assets/app-ui/Fetch/apiFetch.ts', 'assets/app-ui/Auth/authFetch.ts'],
    rules: {
      'no-restricted-imports': ['error', {
        paths: [{
          name: '@/app-ui/Fetch/apiFetchCore',
          message: 'Import apiFetch (public) or authFetch (protected) instead — apiFetchCore is the unguarded implementation behind the facades.',
        }],
      }],
    },
  },

  // --- Generated translation catalogs ---
  // max-lines is a ratchet on HAND-WRITTEN source; these files are emitted by
  // `make i18n-gen` from locale/*.json and nobody may edit them, so the only
  // way to satisfy the limit would be to delete translations. Czech hits it
  // first (a plural block costs one/few/many/other against English's two).
  // Every other rule still applies.
  {
    files: ['assets/app-ui/I18n/catalog/*.ts'],
    rules: {
      'max-lines': 'off',
    },
  },

  // --- Ignored paths ---
  {
    ignores: ['public/**', 'node_modules/**', '*.config.*'],
  },
)
