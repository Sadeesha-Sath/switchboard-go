import { defineConfig } from 'vite';

const proxyTarget = 'http://127.0.0.1:8495';

export default defineConfig({
  base: '/dashboard/',
  build: {
    outDir: 'dist',
  },
  server: {
    proxy: {
      '/usage': proxyTarget,
      '/v1': proxyTarget,
      '/admin': proxyTarget,
      '/metrics': proxyTarget,
      '/dashboard/api': proxyTarget,
    },
  },
});
