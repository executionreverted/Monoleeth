export function renderToast(root: HTMLElement, message: string | null): void {
  root.textContent = message ?? '';
  root.toggleAttribute('data-visible', Boolean(message));
}
