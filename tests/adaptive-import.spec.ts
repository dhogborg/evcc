import { test, expect } from "@playwright/test";
import { start, stop, restart, baseUrl } from "./evcc";
import { expectModalHidden, expectModalVisible } from "./utils";

test.use({ baseURL: baseUrl() });
test.beforeEach(async () => {
  await start("adaptive-import.evcc.yaml");
});
test.afterEach(async () => {
  await stop();
});

test("hourly budget settings and restart", async ({ page, request }) => {
  await page.goto("/#/config");
  await page.getByTestId("generalconfig-hourlyimport").getByRole("button").click();
  const modal = page.getByTestId("hourly-import-modal");
  await expectModalVisible(modal);
  await expect(modal.getByLabel("Maximum imported energy per clock hour")).toBeDisabled();
  await modal.getByLabel("Enable hourly import limit").check();
  await modal.getByLabel("Maximum imported energy per clock hour").fill("14");
  await modal.getByRole("button", { name: "Save", exact: true }).click();
  await expectModalHidden(modal);
  await expect
    .poll(async () => (await (await request.get("/api/state")).json()).hourlyImportLimit)
    .toBe(14);
  await expect
    .poll(async () => (await (await request.get("/api/state")).json()).loadpoints[0].enabled)
    .toBe(false);

  await restart("adaptive-import.evcc.yaml");
  await page.reload();
  await expect(page.getByTestId("generalconfig-hourlyimport")).toContainText("14");
  expect((await (await request.get("/api/state")).json()).hourlyImportLimit).toBe(14);
  await page.getByTestId("generalconfig-hourlyimport").getByRole("button").click();
  await expectModalVisible(modal);
  await modal.getByLabel("Enable hourly import limit").uncheck();
  await modal.getByRole("button", { name: "Save", exact: true }).click();
  await expect
    .poll(async () => (await (await request.get("/api/state")).json()).hourlyImportLimit)
    .toBe(0);
});

test("loadpoint schedule blocks Smart and allows Fast", async ({ page, request }) => {
  await page.goto("/");
  await page.getByTestId("loadpoint-settings-button").nth(1).click();
  const modal = page.getByTestId("loadpoint-settings-modal");
  await expectModalVisible(modal);
  await modal.getByLabel("Enable Smart-mode restrictions").check();
  const rule = modal.getByRole("group", { name: "Restriction 1", exact: true });
  for (const label of ["Jan", "Feb", "Mar", "Nov", "Dec", "Mon", "Tue", "Wed", "Thu", "Fri"]) {
    await rule.getByLabel(label, { exact: true }).uncheck();
  }
  await rule.getByLabel("Start (inclusive)").fill("00:00");
  await rule.getByLabel("End (exclusive)").fill("23:59");
  await modal.getByRole("button", { name: "Save", exact: true }).click();
  await expect(modal.getByText("Charging restrictions saved.", { exact: true })).toBeVisible();
  await modal.getByRole("button", { name: "Close", exact: true }).click();
  await expectModalHidden(modal);
  await page.getByRole("button", { name: "Smart", exact: true }).click();
  await expect
    .poll(
      async () => (await (await request.get("/api/state")).json()).loadpoints[0].scheduleBlocked
    )
    .toBe(true);
  await expect
    .poll(async () => (await (await request.get("/api/state")).json()).loadpoints[0].enabled)
    .toBe(false);
  await page.getByRole("button", { name: "Fast", exact: true }).click();
  await expect
    .poll(
      async () => (await (await request.get("/api/state")).json()).loadpoints[0].scheduleBlocked
    )
    .toBe(false);
  await expect
    .poll(async () => (await (await request.get("/api/state")).json()).loadpoints[0].enabled)
    .toBe(true);
  await restart("adaptive-import.evcc.yaml");
  const settings = await (await request.get("/api/loadpoints/1/chargingrestrictions")).json();
  expect(settings.enabled).toBe(true);
  expect(settings.rules[0].start).toBe("00:00");
});
