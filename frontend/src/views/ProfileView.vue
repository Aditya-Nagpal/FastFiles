<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useUserStore } from '../stores/user';
import ProfileEditForm from '../components/profile/ProfileEditForm.vue';
import ProfilePhotoEditor from '../components/profile/ProfilePhotoEditor.vue';
import type { UserProfile } from '../stores/user';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';

const userStore = useUserStore();
const isLoading = ref(false);
const errorMessage = ref('');

const isEditing = ref(false);
const isSaving = ref(false);
const updateError = ref('');

const isUploadingPhoto = ref(false);
const isRemovingPhoto = ref(false);
const photoError = ref('');

const authStore = useAuthStore();
const router = useRouter();
const isSigningOut = ref(false);

const profile = computed(() => userStore.user);

async function saveProfile(payload: Partial<UserProfile>) {
    isSaving.value = true;
    updateError.value = '';

    try {
        await userStore.updateUserProfile(payload);
        isEditing.value = false;
    } catch {
        updateError.value = 'Could not update your profile. Please try again.';
    } finally {
        isSaving.value = false;
    }
}

function cancelEditing() {
    updateError.value = '';
    isEditing.value = false;
}

function formatDate(value?: string | null) {
    if (!value) return 'Not provided';

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;

    return new Intl.DateTimeFormat('en-IN', {
        dateStyle: 'long',
    }).format(date);
}

function displayValue(value?: string | number | null) {
    return value === null || value === undefined || value === '' ? 'Not provided' : String(value);
}

async function uploadPhoto(file: File) {
    isUploadingPhoto.value = true;
    photoError.value = '';

    try {
        await userStore.updateDisplayPicture(file);
    } catch {
        photoError.value = 'Could not update your photo. Please try again.';
    } finally {
        isUploadingPhoto.value = false;
    }
}

async function removePhoto() {
    isRemovingPhoto.value = true;
    photoError.value = '';

    try {
        await userStore.removeDisplayPicture();
    } catch {
        photoError.value = 'Could not remove your photo. Please try again.';
    } finally {
        isRemovingPhoto.value = false;
    }
}

async function loadProfile() {
    isLoading.value = true;
    errorMessage.value = '';

    try {
        await userStore.fetchUserProfile();
    } catch {
        errorMessage.value = 'Could not load your profile. Please try again.';
    } finally {
        isLoading.value = false;
    }
}

async function signOut() {
    if (isSigningOut.value) return;

    isSigningOut.value = true;

    try {
        await authStore.logout();
    } catch (error) {
        // The auth store clears the local session even if the logout request fails.
        console.error('Sign-out request failed:', error);
    } finally {
        userStore.clearProfile();
        await router.replace({ name: 'login' });
        isSigningOut.value = false;
    }
}

onMounted(() => {
    if (!profile.value) {
        void loadProfile();
    }
});
</script>

<template>
    <main class="profile-page">
        <header class="profile-page-header">
            <div class="profile-header-actions">
                <RouterLink class="profile-back-link" to="/">
                    ← Back to files
                </RouterLink>

                <button class="profile-signout-button" type="button" :disabled="isSigningOut" @click="signOut">
                    {{ isSigningOut ? 'Signing out…' : 'Sign out' }}
                </button>
            </div>
            <p class="profile-eyebrow">ACCOUNT</p>
            <h1>Your profile</h1>
            <p class="profile-intro">View your account details.</p>
        </header>

        <p v-if="isLoading" class="profile-state" role="status">
            Loading your profile…
        </p>

        <div v-else-if="errorMessage" class="profile-state profile-error" role="alert">
            <p>{{ errorMessage }}</p>
            <button type="button" class="profile-button" @click="loadProfile">
                Try again
            </button>
        </div>

        <section v-else-if="profile" class="profile-card" aria-label="Profile information">
            <div class="profile-summary">
                <ProfilePhotoEditor :image-url="profile.display_picture"
                    :initial="profile.name?.trim().charAt(0).toUpperCase() || 'U'" :is-uploading="isUploadingPhoto"
                    :is-removing="isRemovingPhoto" :error="photoError" @upload="uploadPhoto" @remove="removePhoto" />

                <div class="profile-identity">
                    <h2>{{ displayValue(profile.name) }}</h2>
                    <p>{{ displayValue(profile.email) }}</p>
                </div>
            </div>

            <div class="profile-section-heading">
                <h2>Personal details</h2>
                <button v-if="!isEditing" class="profile-edit-button" type="button" @click="isEditing = true">
                    Edit profile
                </button>
            </div>

            <ProfileEditForm v-if="isEditing" :profile="profile" :is-saving="isSaving" :error="updateError"
                @save="saveProfile" @cancel="cancelEditing" />

            <dl v-else class="profile-details">
                <div class="profile-detail">
                    <dt>Alternate email</dt>
                    <dd>{{ displayValue(profile.alternate_email) }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Contact number</dt>
                    <dd>{{ displayValue(profile.contact_number) }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Country</dt>
                    <dd>{{ displayValue(profile.country) }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Date of birth</dt>
                    <dd>{{ formatDate(profile.dob) }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Age</dt>
                    <dd>{{ profile.age == null ? 'Not provided' : `${profile.age} years` }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Gender</dt>
                    <dd>{{ displayValue(profile.gender) }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Timezone</dt>
                    <dd>{{ displayValue(profile.timezone) }}</dd>
                </div>
                <div class="profile-detail">
                    <dt>Plan</dt>
                    <dd>{{ displayValue(profile.plan) }}</dd>
                </div>
                <div class="profile-detail profile-detail-about">
                    <dt>About</dt>
                    <dd>{{ displayValue(profile.about) }}</dd>
                </div>
            </dl>
        </section>
    </main>
</template>

<style src="../assets/styles/ProfileView.css" scoped></style>