import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: process.env.NEST_GATEWAY_URL ?? 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
    // Vitest owns src/ unit tests; e2e/ is Playwright's (uses @playwright/test).
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
    server: {
      deps: {
        // react-libs' dist/index.js re-exports via a directory import ('./components'),
        // which Node's ESM resolver rejects. Inlining routes it through Vite's
        // resolver (same as the production build) instead of Node's.
        inline: ['@penguintechinc/react-libs'],
      },
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/main.tsx',
        'src/server.js',
        'src/test/**',
        '**/*.d.ts',
      ],
    },
  },
});
