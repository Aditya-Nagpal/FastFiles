<template>
  <div class="user-profile-page py-5">
    <div class="user-profile-card">
      <h2 class="text-center mb-5 fw-semibold">User Profile</h2>

      <div v-if="user">
        <div class="d-flex flex-column flex-md-row align-items-center mb-5 profile-header">
          <div class="mb-4 mb-md-0 me-md-5 text-center">
            <img
              :src="displayPictureUrl"
              alt="Display Picture"
              class="rounded-circle mb-2 profile-dp"
              width="120"
              height="120"
            />

            <div v-if="!isChangingDP" class="mt-2">
              <button
                class="btn btn-sm btn-outline-primary me-2 dp-button"
                @click="triggerDPChange"
              >
                {{ user.display_picture ? 'Change Photo' : 'Upload Photo' }}
              </button>
              <button
                v-if="user.display_picture"
                class="btn btn-sm btn-outline-danger dp-button"
                @click="removeDP"
              >
                Remove Photo
              </button>
            </div>

            <div v-else>
              <input type="file" @change="handleFileChange" class="form-control form-control-sm mt-2" id="dp-input" />
              <button class="btn btn-sm btn-success mt-2 me-2 dp-button" :disabled="!dpInput" @click="uploadDP">Save</button>
              <button class="btn btn-sm btn-secondary mt-2 dp-button" @click="cancelDPChange">Cancel</button>
            </div>
          </div>

          <div class="flex-grow-1 header-details">
            <div class="d-flex justify-content-between align-items-center mb-3">
              <span class="detail-label">Email:</span>
              <span class="detail-value text-break">{{ user.email }}</span>
            </div>
            <div class="d-flex justify-content-between align-items-center mb-3">
              <span class="detail-label">Date of Birth:</span>
              <span class="detail-value">{{ formatDate(user.dob) }}</span>
            </div>
            <div class="d-flex justify-content-between align-items-center">
              <span class="detail-label">Age:</span>
              <span class="detail-value">{{ user.age }} years</span>
            </div>
          </div>
        </div>

        <hr class="mb-5"/>

        <div class="row g-4 profile-form-grid">
          
          <div class="col-md-6 form-group">
            <label class="form-label" for="name">Name *</label>
            <input
              type="text"
              class="form-control"
              v-model="form.name"
              :disabled="!isEditing"
              required
              id="name"
              name="name"
            />
          </div>

          <div class="col-md-6 form-group">
            <label class="form-label" for="alternate_email">Alternate Email</label>
            <input
              type="email"
              class="form-control"
              v-model="form.alternate_email"
              :disabled="!isEditing"
              id="alternate_email"
              name="alternate_email"
            />
          </div>

          <div class="col-md-6 form-group">
            <label class="form-label" for="contact_number">Contact Number</label>
            <div class="input-group" id="contact-group">
              <button
                class="btn btn-outline-secondary d-flex align-items-center gap-2 country-btn"
                type="button"
                data-bs-toggle="dropdown"
                aria-expanded="false"
                :disabled="!isEditing"
                aria-label="Select Country Code"
              >
                <img :src="selectedCountry.flag" alt="flag" width="22" height="15" />
                <span>{{ selectedCountry.code }}</span>
              </button>
              <ul class="dropdown-menu" style="max-height: 300px; overflow-y: auto;">
                <li
                  v-for="country in countries"
                  :key="country.name"
                  @click="isEditing && selectCountry(country)"
                  class="dropdown-item d-flex align-items-center gap-2"
                >
                  <img :src="country.flag" alt="flag" width="22" height="15" />
                  <span>{{ country.name }} ({{ country.code }})</span>
                </li>
              </ul>
              <input
                v-model="form.contact_number"
                type="tel"
                class="form-control"
                :disabled="!isEditing"
                maxlength="10"
                @keypress="isNumberKey"
                @input="formatContactNumber"
                id="contact_number"
                name="contact_number"
                aria-describedby="contact-group"
              />
            </div>
          </div>
          
          <div class="col-md-6 form-group">
            <label class="form-label" for="gender">Gender</label>
            <select v-model="form.gender" class="form-select" :disabled="!isEditing" id="gender" name="gender">
              <option disabled value="">Select gender</option>
              <option v-for="g in genderOptions" :key="g" :value="g">{{ processGenderKey(g) }}</option>
            </select>
          </div>

          <div class="col-md-6 form-group">
            <label class="form-label" for="timezone">Timezone</label>
            <select v-model="form.timezone" class="form-select" :disabled="!isEditing" id="timezone" name="timezone">
              <option disabled value="">Select timezone</option>
              <option v-for="t in timezones" :key="t" :value="t">{{ t }}</option>
            </select>
          </div>

          <div class="col-md-6 form-group">
            <label class="form-label" for="plan">Plan</label>
            <select v-model="form.plan" class="form-select" :disabled="!isEditing" id="plan" name="plan">
              <option disabled value="">Select plan</option>
              <option v-for="p in planOptions" :key="p" :value="p">{{ p }}</option>
            </select>
          </div>
          
          <div class="col-12 form-group">
            <label class="form-label" for="about">About</label>
            <textarea
              v-model="form.about"
              rows="3"
              class="form-control"
              placeholder="Tell us about yourself..."
              :disabled="!isEditing"
              :maxlength="MAX_ABOUT_CHARS"
              id="about"
              name="about"
            ></textarea>
            <div class="char-count-display">
              {{ form.about?.length || 0 }} / {{ MAX_ABOUT_CHARS }} characters
            </div>
          </div>

        </div>

        <div class="text-center mt-5 profile-actions">
          <button
            class="btn btn-primary me-2 px-4"
            v-if="isEditing"
            @click="updateProfile"
            :disabled="!isFormValid"
          >
            Save Changes
          </button>
          <button
            class="btn btn-secondary px-4"
            v-if="isEditing"
            @click="cancelEdit"
          >
            Cancel
          </button>
          <button
            class="btn btn-outline-primary px-4"
            v-else
            @click="isEditing = true"
          >
            Edit Info
          </button>
        </div>
      </div>

      <div v-else class="text-center py-5">
        <p>Loading user data...</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, computed, reactive, ref, watch } from 'vue';
import { useUserStore } from '../store/user';
import defaultImage from '../images/person.png';
import { toast } from 'vue3-toastify';

const userStore = useUserStore();

const MAX_ABOUT_CHARS = 500;
const INITIAL_COUNTRY_CODE = '+91';

const user = computed(() => userStore.user);
const isEditing = ref(false);
const isChangingDP = ref(false);
const dpInput = ref(null);

const form = reactive({
  name: '',
  alternate_email: '',
  contact_number: '',
  gender: '',
  timezone: '',
  plan: '',
  about: '',
});

const countries = ref([]);
const timezones = ref([]);
const selectedCountry = ref({ name: 'India', code: INITIAL_COUNTRY_CODE, flag: 'https://flagcdn.com/w40/in.png' });

const genderOptions = ['male', 'female', 'other', 'prefer_not_to_say'];
const planOptions = ['FREE', 'PRO', 'BUSINESS'];

const displayPictureUrl = computed(() => {
  return user.value?.display_picture || defaultImage; 
});

const isFormValid = computed(() => {
  if (!isEditing.value) return true;
  return !!form.name;
});

const processGenderKey = (g) => {
  if (g) {
    let G = g.split('_').map((word) => word.charAt(0).toUpperCase() + word.slice(1));
    return G.join(' ');
  }
  return '';
};

const formatDate = (dateString) => {
    if (!dateString) return 'N/A';
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' });
};

const isNumberKey = (evt) => {
  const charCode = (evt.which) ? evt.which : evt.keyCode;
  if (charCode > 31 && (charCode < 48 || charCode > 57)) {
    evt.preventDefault();
  }
  return true;
};

const formatContactNumber = () => {
    let cleaned = form.contact_number.replace(/\D/g, '');
    form.contact_number = cleaned.slice(0, 10);
};

const fetchInitialData = async () => {
    try {
        const res = await fetch(
          'https://api.restcountries.com/countries/v5?response_fields=names,flag.url_png,calling_codes',
          { headers: { 'Authorization': `Bearer ${import.meta.env.VITE_RESTCOUNTRIES_API_KEY}` } }
        );
        const data = await res.json();
        countries.value = data?.data?.objects
            ?.map(country => ({
                name: country.names.common,
                code: country.calling_codes
                    ? `+${country.calling_codes?.length > 0 ? country.calling_codes[0] : ''}`
                    : '',
                flag: country.flag.url_png || '',
            }))
            .filter(country => country.flag)
            .sort((a, b) => a.name.localeCompare(b.name));
    } catch (error) {
        console.error('Error fetching countries:', error);
    }
    
    timezones.value = Intl.supportedValuesOf('timeZone').sort();
};

const initializeForm = (userData) => {
  if (!userData) return;

  form.name = userData.name || '';
  form.alternate_email = userData.alternate_email || '';
  form.gender = userData.gender || '';
  form.timezone = userData.timezone || '';
  form.plan = userData.plan || '';
  form.about = userData.about || '';
  
  if (userData.contact_number) {
    const codeMatch = countries.value.find(c => userData.contact_number.startsWith(c.code));
    
    if (codeMatch) {
      selectedCountry.value = codeMatch; 
      
      const localNumber = userData.contact_number.substring(codeMatch.code.length);
      form.contact_number = localNumber.replace(/\D/g, '').slice(-10);
    } else {
      const fullDigits = userData.contact_number.replace(/\D/g, ''); 
      form.contact_number = fullDigits.slice(-10); 
      
      selectedCountry.value = countries.value.find(c => c.code === INITIAL_COUNTRY_CODE) || selectedCountry.value;
    }
  } else {
    selectedCountry.value = countries.value.find(c => c.code === INITIAL_COUNTRY_CODE) || selectedCountry.value;
    form.contact_number = '';
  }
};

onMounted(fetchInitialData);

watch([user, countries], ([newUser, newCountries]) => {
    if (newUser && newCountries.length > 0) {
        initializeForm(newUser);
    }
}, { immediate: true });


const selectCountry = (country) => {
  selectedCountry.value = country;
};

const triggerDPChange = () => {
  isChangingDP.value = true;
};

const cancelDPChange = () => {
  dpInput.value = null;
  isChangingDP.value = false;
};

const handleFileChange = (e) => {
  dpInput.value = e.target.files?.[0];
};

const uploadDP = async () => {
  const file = dpInput.value;
  if (!file) {
    toast.info('No file selected');
    return;
  }

  const formData = new FormData();
  formData.append('displayPicture', file); 

  try {
    await userStore.updateDisplayPicture(formData);
    toast.success('Photo updated successfully');
    isChangingDP.value = false;
    dpInput.value = null; 
  } catch (error) {
    console.error('DP upload failed', error);
    toast.error('Failed to update display picture');
  }
};

const removeDP = async () => {
  try {
    await userStore.removeDisplayPicture();
    toast.success('Display picture removed');
  } catch (err) {
    console.error('Remove DP failed', err);
    toast.error('Failed to remove display picture');
  }
};

const updateProfile = async () => {
  if (!isFormValid.value) {
    toast.error('Please ensure all required fields are filled.');
    return;
  }
  
  const originalUser = user.value;
  const payload = {};
  
  const editableKeys = ['name', 'alternate_email', 'gender', 'timezone', 'plan', 'about'];
  
  editableKeys.forEach(key => {
    if (form[key] !== originalUser[key] && form[key] !== null) {
      payload[key] = form[key];
    }
  });

  const newFullContact = selectedCountry.value.code + form.contact_number;
  
  if (originalUser.contact_number !== newFullContact) { 
    if (form.contact_number.length === 10) { 
        payload.contact_number = newFullContact; 
    } else if (form.contact_number.length === 0) {
        payload.contact_number = ''; 
    } else {
        toast.error('Contact number must be 10 digits long or empty.');
        return;
    }
  }
  
  if (originalUser.country !== selectedCountry.value.name) {
      payload.country = selectedCountry.value.name;
  }

  if (Object.keys(payload).length === 0) {
    toast.info('No changes detected.');
    isEditing.value = false;
    return;
  }

  try {
    await userStore.updateUserProfile(payload);
    toast.success('Profile updated');
    isEditing.value = false;
    initializeForm(userStore.user); 
  } catch (err) {
    console.error('Update failed', err);
    toast.error(err.response?.data?.message || 'Failed to update profile');
  }
};

const cancelEdit = () => {
  initializeForm(user.value);
  isEditing.value = false;
};
</script>

<style scoped>
.user-profile-page {
  background-color: #fcfcfd; 
  min-height: 100vh;
  padding-bottom: 5rem !important; 
}

.user-profile-card {
  width: 90%;
  max-width: 950px; 
  background: white;
  border-radius: 1rem;
  padding: 3rem; 
  margin: 0 auto;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.05); 
}

.profile-dp {
    border: 3px solid #6366f1;
    object-fit: cover;
}

.header-details {
    padding-left: 1rem;
    padding-right: 1rem;
    font-size: 0.95rem;
}
.detail-label {
    font-weight: 600;
    color: #4b5563; 
}
.detail-value {
    color: #1f2937; 
    font-weight: 500;
    text-align: right;
}

.form-group {
    margin-bottom: 1.5rem;
}

.form-control,
.form-select,
textarea {
  width: 100%;
  border-color: #e2e8f0; 
  border-radius: 0.5rem;
  transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
}

.form-control:focus,
.form-select:focus,
textarea:focus {
  border-color: #6366f1; 
  box-shadow: 0 0 0 0.25rem rgba(99, 102, 241, 0.25);
}

.form-control:disabled,
.form-select:disabled,
textarea:disabled {
    background-color: #f9fafb;
    color: #4b5563;
    opacity: 1;
}

label {
  font-weight: 600 !important; 
  margin-bottom: 0.4rem;
  font-size: 0.95rem; 
  color: #333;
}

.country-btn {
  border-right: 0;
  border-color: #e2e8f0;
  background-color: #f8f8f8 !important; 
  padding-left: 0.75rem;
  padding-right: 0.75rem;
  border-radius: 0.5rem 0 0 0.5rem;
}

.country-btn:hover:not(:disabled),
.country-btn:focus:not(:disabled),
.country-btn:active:not(:disabled),
.country-btn:focus,
.country-btn:active {
  box-shadow: none !important;
}

.input-group > .form-control[disabled],
.country-btn[disabled] {
    border-color: #e2e8f0 !important;
    box-shadow: none !important;
    outline: none !important;
}

.country-btn[disabled] {
    background-color: #e5e7eb !important;
    color: #6b7280;
    opacity: 1;
}

.char-count-display {
    text-align: right;
    font-size: 0.85rem;
    color: #6b7280;
    margin-top: 0.25rem;
    padding-right: 0.25rem;
}

.btn {
    border-radius: 0.5rem;
    font-weight: 600;
}
.btn-primary {
  background-color: #6366f1;
  border-color: #6366f1;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1); 
}
.btn-primary:hover {
  background-color: #4f46e5;
  border-color: #4f46e5;
}
.btn-outline-primary {
    color: #6366f1;
    border-color: #6366f1;
}

.dp-button {
    font-size: 0.8rem;
    padding: 0.3rem 0.6rem;
}

@media (max-width: 768px) {
  .user-profile-card {
    padding: 2rem 1.5rem;
    width: 95%;
  }
  .profile-header {
      flex-direction: column;
      align-items: flex-start !important;
  }
  .profile-header .header-details {
      padding: 0;
      width: 100%;
  }
}
</style>