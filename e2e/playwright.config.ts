import { defineConfig, devices } from "@playwright/test";

// Ports are remapped by docker-compose.playwright.yml so the stack can run
// alongside other projects holding :3000/:8080.
//   frontend -> http://localhost:3001   api -> http://localhost:8090
const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:3001";
export const API_URL = process.env.E2E_API_URL ?? "http://localhost:8090";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: [["list"]],
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: BASE_URL,
    trace: "retain-on-failure",
    // The dev server (next dev) compiles a route on its first hit, which can take
    // several seconds; give navigations room before failing.
    navigationTimeout: 20_000,
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
      testIgnore: /mobile\.spec\.ts/,
    },
    // The desktop suite covers functional behavior; the mobile project runs
    // only mobile.spec.ts, which checks layout on a phone viewport.
    // Pixel 5, not an iPhone profile: iPhone emulation needs the WebKit build,
    // and layout/overflow questions are engine-independent enough that the
    // already-installed Chromium answers them.
    {
      name: "mobile",
      use: { ...devices["Pixel 5"] },
      testMatch: /mobile\.spec\.ts/,
    },
  ],
});
