<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import type { UserProfile } from '../../stores/user';

interface Country {
    name: string;
    code: string;
    flag: string;
}

interface CountriesApiCountry {
    names?: { common?: string };
    calling_codes?: string[];
    flag?: { url_png?: string };
}

const props = defineProps<{
    profile: UserProfile;
    isSaving: boolean;
    error: string;
}>();

const emit = defineEmits<{
    (event: 'save', payload: Partial<UserProfile>): void;
    (event: 'cancel'): void;
}>();

const INITIAL_COUNTRY_CODE = '+91';

const countries = ref<Country[]>([]);
const timezones = ref<string[]>([]);
const selectedCountryName = ref('India');
const phoneError = ref('');

const form = reactive({
    name: '',
    alternate_email: '',
    contact_number: '',
    gender: '',
    timezone: '',
    plan: '',
    about: '',
});

const countryPicker = ref<HTMLDetailsElement | null>(null);

function selectCountry(country: Country) {
    selectedCountryName.value = country.name;
    if (countryPicker.value) countryPicker.value.open = false;
}

const selectedCountry = computed(
    () => countries.value.find((country) => country.name === selectedCountryName.value)
        ?? { name: 'India', code: INITIAL_COUNTRY_CODE, flag: '' },
);

const localError = computed(() => {
    if (!form.name.trim()) return 'Name is required.';
    return phoneError.value;
});

function initializeForm(profile: UserProfile) {
    form.name = profile.name ?? '';
    form.alternate_email = profile.alternate_email ?? '';
    form.gender = profile.gender ?? '';
    form.timezone = profile.timezone ?? '';
    form.plan = profile.plan ?? '';
    form.about = profile.about ?? '';

    const savedNumber = profile.contact_number ?? '';
    const matchingCountry = countries.value
        .filter((country) => savedNumber.startsWith(country.code))
        .sort((a, b) => b.code.length - a.code.length)[0];

    if (matchingCountry) {
        selectedCountryName.value = matchingCountry.name;
        form.contact_number = savedNumber
            .slice(matchingCountry.code.length)
            .replace(/\D/g, '')
            .slice(-10);
    } else {
        selectedCountryName.value =
            countries.value.find((country) => country.name === profile.country)?.name
            ?? countries.value.find((country) => country.code === INITIAL_COUNTRY_CODE)?.name
            ?? 'India';

        form.contact_number = savedNumber.replace(/\D/g, '').slice(-10);
    }
}

watch(
    [() => props.profile, countries],
    ([profile, availableCountries]) => {
        if (profile && availableCountries.length > 0) {
            initializeForm(profile);
        }
    },
    { immediate: true },
);

async function loadInitialData() {
    timezones.value = Intl.supportedValuesOf('timeZone').sort();

    const apiKey = import.meta.env.VITE_RESTCOUNTRIES_API_KEY;
    if (!apiKey) {
        console.error('Missing VITE_RESTCOUNTRIES_API_KEY in the frontend environment.');
        return;
    }

    try {
        const response = await fetch(
            'https://api.restcountries.com/countries/v5?response_fields=names,flag.url_png,calling_codes',
            {
                headers: {
                    Authorization: `Bearer ${apiKey}`,
                },
            },
        );

        if (!response.ok) {
            throw new Error(`Country lookup failed (${response.status}).`);
        }

        const data = await response.json() as {
            data?: { objects?: CountriesApiCountry[] };
        };

        countries.value = (data.data?.objects ?? [])
            .map((country) => ({
                name: country.names?.common ?? '',
                code: country.calling_codes?.[0]
                    ? `+${country.calling_codes[0]}`
                    : '',
                flag: country.flag?.url_png ?? '',
            }))
            .filter((country) => country.name && country.code && country.flag)
            .sort((a, b) => a.name.localeCompare(b.name));

        if (!countries.value.some((country) => country.name === selectedCountryName.value)) {
            selectedCountryName.value =
                countries.value.find((country) => country.code === INITIAL_COUNTRY_CODE)?.name
                ?? countries.value[0]?.name
                ?? 'India';
        }
    } catch (error) {
        console.error('Could not load country calling codes:', error);
    }
}

function handlePhoneInput(event: Event) {
    const input = event.target as HTMLInputElement;
    form.contact_number = input.value.replace(/\D/g, '').slice(0, 10);
    phoneError.value = '';
}

function submitForm() {
    if (props.isSaving) return;

    if (form.contact_number && form.contact_number.length !== 10) {
        phoneError.value = 'Enter a 10-digit phone number, or leave it empty.';
        return;
    }

    phoneError.value = '';

    emit('save', {
        name: form.name.trim(),
        alternate_email: form.alternate_email.trim(),
        contact_number: form.contact_number
            ? `${selectedCountry.value.code}${form.contact_number}`
            : '',
        country: selectedCountry.value.name,
        gender: form.gender,
        timezone: form.timezone,
        plan: form.plan,
        about: form.about.trim(),
    });
}

onMounted(loadInitialData);
</script>

<template>
    <form class="profile-edit-form" @submit.prevent="submitForm">
        <div class="profile-edit-grid">
            <label class="profile-field">
                <span>Name *</span>
                <input v-model="form.name" type="text" maxlength="120" required />
            </label>

            <label class="profile-field">
                <span>Alternate email</span>
                <input v-model="form.alternate_email" type="email" />
            </label>

            <label class="profile-field">
                <span>Contact number</span>

                <div class="profile-phone-input">
                    <details ref="countryPicker" class="country-code-picker">
                        <summary class="country-code-trigger" aria-label="Choose country calling code">
                            <img v-if="selectedCountry.flag" :src="selectedCountry.flag" :alt="selectedCountry.name" />
                            <span>{{ selectedCountry.code }}</span>
                            <span class="country-code-chevron" aria-hidden="true">⌄</span>
                        </summary>

                        <div class="country-code-menu" role="list">
                            <button v-for="country in countries" :key="country.name" class="country-option"
                                type="button" @click="selectCountry(country)">
                                <img :src="country.flag" :alt="country.name" />
                                <span class="country-option-name">{{ country.name }}</span>
                                <span class="country-option-code">{{ country.code }}</span>
                            </button>
                        </div>
                    </details>

                    <input class="phone-number-input" :value="form.contact_number" type="tel" inputmode="numeric"
                        maxlength="10" placeholder="Phone number" aria-label="Phone number" @input="handlePhoneInput" />
                </div>
            </label>

            <label class="profile-field">
                <span>Gender</span>
                <select v-model="form.gender">
                    <option value="">Select gender</option>
                    <option value="male">Male</option>
                    <option value="female">Female</option>
                    <option value="other">Other</option>
                    <option value="prefer_not_to_say">Prefer not to say</option>
                </select>
            </label>

            <label class="profile-field">
                <span>Timezone</span>
                <select v-model="form.timezone">
                    <option value="">Select timezone</option>
                    <option v-for="timezone in timezones" :key="timezone" :value="timezone">
                        {{ timezone }}
                    </option>
                </select>
            </label>

            <label class="profile-field">
                <span>Plan</span>
                <input :value="form.plan || 'Not set'" type="text" disabled />
            </label>

            <label class="profile-field profile-field-about">
                <span>About</span>
                <textarea v-model="form.about" rows="4" maxlength="500" placeholder="Tell us about yourself" />
                <small>{{ form.about.length }} / 500 characters</small>
            </label>
        </div>

        <p v-if="localError" class="profile-form-error" role="alert">
            {{ localError }}
        </p>
        <p v-else-if="props.error" class="profile-form-error" role="alert">
            {{ props.error }}
        </p>

        <div class="profile-form-actions">
            <button class="profile-cancel-button" type="button" :disabled="props.isSaving" @click="emit('cancel')">
                Cancel
            </button>
            <button class="profile-save-button" type="submit" :disabled="props.isSaving || Boolean(localError)">
                {{ props.isSaving ? 'Saving…' : 'Save changes' }}
            </button>
        </div>
    </form>
</template>

<style src="../../assets/styles/ProfileEditForm.css" scoped></style>