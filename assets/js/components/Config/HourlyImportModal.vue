<template>
	<GenericModal
		id="hourlyImportModal"
		ref="modal"
		:title="$t('config.hourlyImport.title')"
		config-modal-name="hourlyImport"
		data-testid="hourly-import-modal"
		@open="open"
	>
		<p>{{ $t("config.hourlyImport.description") }}</p>
		<p class="small text-muted">{{ $t("config.hourlyImport.caution") }}</p>
		<form @submit.prevent="save">
			<div class="form-check form-switch mb-3">
				<input
					id="hourlyImportEnabled"
					v-model="enabled"
					type="checkbox"
					class="form-check-input"
				/>
				<label for="hourlyImportEnabled" class="form-check-label">{{
					$t("config.hourlyImport.enabled")
				}}</label>
			</div>
			<label for="hourlyImportLimit" class="form-label">{{
				$t("config.hourlyImport.limit")
			}}</label>
			<div class="input-group mb-3">
				<input
					id="hourlyImportLimit"
					v-model.number="limit"
					type="number"
					min="0.1"
					max="1000000"
					step="0.1"
					:disabled="!enabled"
					required
					class="form-control"
					aria-describedby="hourlyImportUnit"
				/>
				<span id="hourlyImportUnit" class="input-group-text">kWh</span>
			</div>
			<div v-if="status && enabled" class="small mb-3" role="status">
				<p>
					{{
						$t("config.hourlyImport.readings", {
							used: fmtNumber(status.used, 2),
							remaining: fmtNumber(status.remaining, 2),
							power: fmtNumber(status.allowedPower / 1000, 1),
						})
					}}
				</p>
				<p>{{ $t(`config.hourlyImport.status.${status.status}`) }}</p>
			</div>
			<p v-if="error" class="text-danger" role="alert">{{ error }}</p>
			<div class="d-flex justify-content-between mt-4">
				<button type="button" class="btn btn-link text-muted" data-bs-dismiss="modal">
					{{ $t("config.general.cancel") }}
				</button>
				<button type="submit" class="btn btn-primary" :disabled="saving">
					{{ $t("config.general.save") }}
				</button>
			</div>
		</form>
	</GenericModal>
</template>
<script lang="ts">
import { defineComponent } from "vue";
import GenericModal from "../Helper/GenericModal.vue";
import formatter from "@/mixins/formatter";
import store from "@/store";
import api from "@/api";
export default defineComponent({
	components: { GenericModal },
	mixins: [formatter],
	data() {
		return { enabled: false, limit: 14, saving: false, error: "" };
	},
	computed: {
		status() {
			return store.state?.hourlyImport;
		},
	},
	methods: {
		open() {
			const value = store.state?.hourlyImportLimit || 0;
			this.enabled = value > 0;
			this.limit = value || 14;
			this.error = "";
		},
		async save() {
			this.saving = true;
			this.error = "";
			try {
				await api.post(`/hourlyimportlimit/${this.enabled ? this.limit : 0}`);
				(this.$refs["modal"] as unknown as InstanceType<typeof GenericModal>).close();
			} catch (e) {
				this.error = e instanceof Error ? e.message : String(e);
			} finally {
				this.saving = false;
			}
		},
	},
});
</script>
