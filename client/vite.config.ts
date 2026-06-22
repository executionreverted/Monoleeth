import { defineConfig } from 'vite';

const proxyTarget = normalizeProxyTarget(process.env.GAME_CLIENT_PROXY_TARGET ?? 'http://127.0.0.1:8080');
const wsProxyTarget = websocketTargetFor(proxyTarget);

export default defineConfig({
  server: {
    host: '127.0.0.1',
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
      },
      '/ws': {
        target: wsProxyTarget,
        ws: true,
      },
    },
  },
  build: {
    target: 'es2022',
  },
});

function normalizeProxyTarget(target: string): string {
  return target.replace(/\/+$/, '');
}

function websocketTargetFor(target: string): string {
  if (target.startsWith('https://')) {
    return `wss://${target.slice('https://'.length)}`;
  }
  if (target.startsWith('http://')) {
    return `ws://${target.slice('http://'.length)}`;
  }
  return target;
}
