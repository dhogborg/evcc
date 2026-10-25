<template>
	<section class="mt-4">
		<h6>{{ $t("main.chargingRestrictions.title") }}</h6>
		<p class="small text-muted">{{ $t("main.chargingRestrictions.description") }}</p>
		<form @submit.prevent="save">
			<div class="form-check form-switch mb-3">
				<input
					:id="`${prefix}-enabled`"
					v-model="draft.enabled"
					type="checkbox"
					class="form-check-input"
				/>
				<label :for="`${prefix}-enabled`" class="form-check-label">{{
					$t("main.chargingRestrictions.enabled")
				}}</label>
			</div>
			<fieldset :disabled="!draft.enabled || saving">
				<label :for="`${prefix}-timezone`" class="form-label">{{
					$t("main.chargingRestrictions.timezone")
				}}</label>
				<input
					:id="`${prefix}-timezone`"
					v-model="draft.timezone"
					class="form-control mb-3"
					required
				/>
				<fieldset
					v-for="(rule, index) in draft.rules"
					:key="index"
					class="border rounded p-3 mb-3"
				>
					<legend class="float-none w-auto fs-6">
						{{ $t("main.chargingRestrictions.rule", { number: index + 1 }) }}
					</legend>
					<p class="small">{{ $t("main.chargingRestrictions.matches") }}</p>
					<div class="mb-3">
						<span class="d-block mb-1">{{
							$t("main.chargingRestrictions.months")
						}}</span>
						<label
							v-for="month in months"
							:key="month.value"
							class="form-check form-check-inline"
						>
							<input
								v-model="rule.months"
								type="checkbox"
								class="form-check-input"
								:value="month.value"
							/>
							<span class="form-check-label">{{ month.name }}</span>
						</label>
					</div>
					<div class="mb-3">
						<span class="d-block mb-1">{{
							$t("main.chargingRestrictions.weekdays")
						}}</span>
						<label
							v-for="day in weekdays"
							:key="day.value"
							class="form-check form-check-inline"
						>
							<input
								v-model="rule.weekdays"
								type="checkbox"
								class="form-check-input"
								:value="day.value"
							/>
							<span class="form-check-label">{{ day.name }}</span>
						</label>
					</div>
					<div class="d-flex gap-3 mb-3">
						<div>
							<label :for="`${prefix}-${index}-start`" class="form-label">{{
								$t("main.chargingRestrictions.start")
							}}</label>
							<input
								:id="`${prefix}-${index}-start`"
								v-model="rule.start"
								type="time"
								class="form-control"
								required
							/>
						</div>
						<div>
							<label :for="`${prefix}-${index}-end`" class="form-label">{{
								$t("main.chargingRestrictions.end")
							}}</label>
							<input
								:id="`${prefix}-${index}-end`"
								v-model="rule.end"
								type="time"
								class="form-control"
								required
							/>
						</div>
					</div>
					<button
						type="button"
						class="btn btn-sm btn-outline-danger"
						@click="draft.rules.splice(index, 1)"
					>
						{{ $t("main.chargingRestrictions.remove") }}
					</button>
				</fieldset>
				<button
					type="button"
					class="btn btn-sm btn-outline-secondary mb-3"
					:disabled="draft.rules.length >= 32"
					@click="addRule"
				>
					{{ $t("main.chargingRestrictions.add") }}
				</button>
			</fieldset>
			<p v-if="blocked" class="text-warning" role="status">
				{{ $t("main.chargingRestrictions.blocked") }}
			</p>
			<p v-if="nextTransition" class="small text-muted">
				{{ $t("main.chargingRestrictions.next", { time: transitionLabel }) }}
			</p>
			<p v-if="error" class="text-danger" role="alert">{{ error }}</p>
			<p v-if="saved" class="text-success" role="status">
				{{ $t("main.chargingRestrictions.saved") }}
			</p>
			<button type="submit" class="btn btn-primary" :disabled="saving">
				{{ $t("config.general.save") }}
			</button>
		</form>
	</section>
</template>
<script lang="ts">
import { defineComponent, type PropType } from "vue";
import api from "@/api";
import type { ChargingRestrictions } from "@/types/evcc";
const initial = (): ChargingRestrictions => ({
	enabled: false,
	timezone: "Europe/Stockholm",
	rules: [],
});
export default defineComponent({
	props: {
		id: { type: [Number, String], required: true },
		value: Object as PropType<ChargingRestrictions>,
		blocked: Boolean,
		nextTransition: String,
	},
	data() {
		return { draft: initial(), saving: false, saved: false, error: "" };
	},
	computed: {
		prefix() {
			return `restrictions-${this.id}`;
		},
		months() {
			return Array.from({ length: 12 }, (_, i) => ({
				value: i + 1,
				name: new Intl.DateTimeFormat(this.$i18n.locale, {
					month: "short",
					timeZone: "UTC",
				}).format(new Date(Date.UTC(2024, i, 1))),
			}));
		},
		weekdays() {
			return Array.from({ length: 7 }, (_, i) => ({
				value: i + 1,
				name: new Intl.DateTimeFormat(this.$i18n.locale, {
					weekday: "short",
					timeZone: "UTC",
				}).format(new Date(Date.UTC(2024, 0, i + 1))),
			}));
		},
		transitionLabel() {
			return this.nextTransition
				? new Intl.DateTimeFormat(this.$i18n.locale, {
						dateStyle: "short",
						timeStyle: "short",
						timeZone: this.value?.timezone || "Europe/Stockholm",
					}).format(new Date(this.nextTransition))
				: "";
		},
	},
	watch: {
		value: {
			immediate: true,
			handler(value?: ChargingRestrictions) {
				this.draft = value ? JSON.parse(JSON.stringify(value)) : initial();
				if (!this.draft.rules.length) this.addRule();
			},
		},
	},
	methods: {
		addRule() {
			this.draft.rules.push({
				months: [1, 2, 3, 11, 12],
				weekdays: [1, 2, 3, 4, 5],
				start: "07:00",
				end: "20:00",
			});
		},
		async save() {
			this.error = "";
			this.saved = false;
			this.saving = true;
			try {
				await api.post(`/loadpoints/${this.id}/chargingrestrictions`, this.draft);
				this.saved = true;
			} catch (e) {
				this.error = e instanceof Error ? e.message : String(e);
			} finally {
				this.saving = false;
			}
		},
	},
});
</script>
