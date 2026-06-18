export function createRequestId(prefix = 'request'): string {
  const cryptoAPI = globalThis.crypto;
  if (cryptoAPI && typeof cryptoAPI.randomUUID === 'function') {
    return cryptoAPI.randomUUID();
  }

  const random = Math.random().toString(36).slice(2, 12);
  const time = Date.now().toString(36);
  return `${prefix}-${time}-${random}`;
}
