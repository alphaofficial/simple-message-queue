module.exports = {
  displayName: "SQS Bridge Integration Tests",
  testEnvironment: "node",
  preset: "ts-jest",
  roots: ["<rootDir>/tests"],
  testMatch: ["**/*.test.ts"],
  globalSetup: "<rootDir>/jestGlobalSetup.ts",
  globalTeardown: "<rootDir>/jestGlobalTeardown.ts", 
  setupFilesAfterEnv: ["<rootDir>/jestTestsSetup.ts"],
  testTimeout: 60000, // Container startup time
  collectCoverageFrom: [
    "tests/**/*.ts",
    "!tests/**/*.d.ts"
  ],
  coverageDirectory: "coverage",
  verbose: true,
  forceExit: true,
  detectOpenHandles: true
};