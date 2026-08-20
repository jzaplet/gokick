<script setup lang="ts">
import type { Component } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { authFetch, isAuthenticated } from '@/app-ui/Auth';
import { tm, useI18n } from '@/app-ui/I18n/useI18n';
import { SUPPORTED_LANGS, toLangParam } from '@/app-ui/I18n/lang';
import type { Lang, TranslationKey } from '@/app-ui/I18n/lang';
import { useToast } from '@/app-ui/Toast/useToast';
import type { ChangeLangErrors } from '@/app-ui/I18n/types/ChangeLangErrors';
import type { ChangeLangFormData } from '@/app-ui/I18n/types/ChangeLangFormData';
import FlagCsIcon from '@/app-ui/Icons/FlagCsIcon.vue';
import FlagEnIcon from '@/app-ui/Icons/FlagEnIcon.vue';

const { t, locale, chooseLocale } = useI18n();
const { error } = useToast();
const router = useRouter();
const route = useRoute();

type LangMeta = {
    icon: Component;
    labelKey: TranslationKey;
};

type LangOption = LangMeta & { code: Lang };

// Per-language metadata as a Record keyed by Lang: a new SUPPORTED_LANGS
// member fails vue-tsc right here until it gets an icon + label — the switcher
// is the one UI that must never lag behind the language list.
const langMeta: Record<Lang, LangMeta> = {
    en: { icon: FlagEnIcon, labelKey: 'lang.en' },
    cs: { icon: FlagCsIcon, labelKey: 'lang.cs' },
};

const options: LangOption[] = SUPPORTED_LANGS.map(
    (code): LangOption => ({ code, ...langMeta[code] }),
);

// Persist the choice to the profile when signed in — the preference then
// follows the user to other browsers (users.lang). Failures surface as a
// toast but never block the local switch.
const persistToProfile = async (lang: Lang): Promise<void> => {
    if (isAuthenticated.value === false) {
        return;
    }
    const body: ChangeLangFormData = { lang };
    const result = await authFetch<null, ChangeLangErrors, ChangeLangFormData>(
        'PUT',
        '/api/v1/profile/lang',
        { body },
    );

    if (result.success === false) {
        error(tm(result.data.general) ?? t('lang.save_failed'));
    }
};

// Keep the URL canonical after a switch: the canonical language is bare,
// others carry their prefix (/cs/…). Re-resolves the current route with the
// new lang param, preserving query and hash.
const applyUrlPrefix = async (lang: Lang): Promise<void> => {
    await router.replace({
        params: { ...route.params, lang: toLangParam(lang) },
        query: route.query,
        hash: route.hash,
    });
};

const switchTo = async (lang: Lang): Promise<void> => {
    if (lang === locale.value) {
        return;
    }
    chooseLocale(lang);
    await applyUrlPrefix(lang);
    await persistToProfile(lang);
};
</script>

<template>
    <div
        class="flex items-center gap-2"
        role="group"
        :aria-label="t('profile.language')"
    >
        <!--
            Inactive is desaturated to 70%, not the full 100%: the Czech flag's
            red (#d7141a) and blue (#11457e) are isoluminant to within 0.2, so
            grayscale(1) paints BOTH rgb(62,62,62) and the wedge disappears into
            the lower band. Partial desaturation reads as muted while keeping
            the hue that is the only thing telling those two apart. The label is
            an endonym ("Čeština" in every catalog), so :lang keeps a screen
            reader from pronouncing it with the document language's phonetics.
        -->
        <button
            v-for="option in options"
            :key="option.code"
            type="button"
            :class="[
                'flex cursor-pointer rounded-xs',
                'transition-[filter] duration-200',
                'focus-visible:outline-2 focus-visible:outline-offset-2',
                'focus-visible:outline-blue-600',
                locale === option.code
                    ? 'grayscale-0'
                    : 'grayscale-[70%] hover:grayscale-[25%]',
            ]"
            :aria-label="t(option.labelKey)"
            :aria-pressed="locale === option.code"
            :title="t(option.labelKey)"
            :lang="option.code"
            @click="switchTo(option.code)"
        >
            <!--
                The hairline is on the flag itself, not a padded box behind it:
                the Czech flag's white half would otherwise dissolve into a
                light header and read as a floating triangle.
            -->
            <component
                :is="option.icon"
                class="w-6 h-4 rounded-xs ring-1 ring-black/10"
            />
        </button>
    </div>
</template>
