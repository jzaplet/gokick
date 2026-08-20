<script setup lang="ts">
import type { DashboardResponse } from '@/app/Dashboard/types/DashboardResponse';
import { isDashboardResponse } from '@/app/Dashboard/types/DashboardResponse';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useI18n } from '@/app-ui/I18n';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';

// The one dashboard fetch-and-show card (F-090): the admin and user dashboards
// differ only in the endpoint they read, so both views mount this component
// instead of carrying twin copies of the fetch/spinner/error logic. The DTO
// carries raw data (nickname); the greeting sentence is composed here from
// the catalogs (no server-rendered prose in data DTOs).
const { endpoint } = defineProps<{
    endpoint: string;
}>();

const { error } = useToast();
const { t } = useI18n();

const nickname = ref('');
const isLoading = ref(true);
// Separate from isLoading so a FAILED fetch renders nothing rather than the
// greeting with an empty {nickname} — a complete sentence with the name
// silently missing reads as success once the error toast has dismissed.
const isLoaded = ref(false);

onMounted(async (): Promise<void> => {
    const result = await authFetch<DashboardResponse>('GET', endpoint, { validate: isDashboardResponse });

    isLoading.value = false;

    if (result.success === false) {
        error(t('dashboard.load_failed'));

        return;
    }

    nickname.value = result.data.nickname;
    isLoaded.value = true;
});
</script>

<template>
    <div class="bg-white rounded-lg shadow-md p-6">
        <div
            v-if="isLoading === true"
            class="flex items-center justify-center py-8"
        >
            <Spinner />
        </div>
        <p
            v-else-if="isLoaded === true"
            class="text-gray-700"
        >
            {{ t('dashboard.welcome', { nickname }) }}
        </p>
    </div>
</template>
