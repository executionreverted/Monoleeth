const HUD_INPUT_SUPPRESSION_MS = 220;

interface SpaceWindow extends Window {
  __SPACE_MORPG_HUD_INPUT_UNTIL__?: number;
}

export function markHUDInputSuppressed(durationMs = HUD_INPUT_SUPPRESSION_MS): void {
  if (typeof window === 'undefined' || typeof performance === 'undefined') {
    return;
  }
  (window as SpaceWindow).__SPACE_MORPG_HUD_INPUT_UNTIL__ = performance.now() + durationMs;
}

export function clearHUDInputSuppression(): void {
  if (typeof window === 'undefined') {
    return;
  }
  (window as SpaceWindow).__SPACE_MORPG_HUD_INPUT_UNTIL__ = 0;
}

export function pointerTargetOwnsUI(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && !target.classList.contains('world-canvas') && Boolean(target.closest(pointerBlockingSelector()));
}

export function worldCanvasInputBlocked(activeElement: Element | null = activeDocumentElement()): boolean {
  return hudInputSuppressed() || uiOwnsWorldFocus(activeElement);
}

export function releaseTransientHUDControlFocus(activeElement: Element | null = activeDocumentElement()): boolean {
  if (!(activeElement instanceof HTMLElement) || !isFocusedHUDControl(activeElement)) {
    return false;
  }
  if (persistentUIOwnsWorldFocus(activeElement)) {
    return false;
  }
  activeElement.blur();
  return true;
}

export function worldKeyboardShortcutAllowed(input: {
  eventTarget: EventTarget | null;
  activeElement: Element | null;
  uiOwnsFocus: boolean;
}): boolean {
  if (input.uiOwnsFocus) {
    return false;
  }
  const target = input.eventTarget instanceof HTMLElement ? input.eventTarget : null;
  return !isKeyboardBlockedElement(target) && !isKeyboardBlockedElement(input.activeElement);
}

function hudInputSuppressed(): boolean {
  if (typeof window === 'undefined' || typeof performance === 'undefined') {
    return false;
  }
  return performance.now() < ((window as SpaceWindow).__SPACE_MORPG_HUD_INPUT_UNTIL__ ?? 0);
}

function uiOwnsWorldFocus(activeElement: Element | null): boolean {
  return persistentUIOwnsWorldFocus(activeElement) || isFocusedHUDControl(activeElement);
}

function persistentUIOwnsWorldFocus(activeElement: Element | null): boolean {
  if (isFocusBlockedElement(activeElement)) {
    return true;
  }
  if (typeof document === 'undefined') {
    return false;
  }
  return Boolean(document.querySelector('.hud-modal, [data-modal][role="dialog"], [data-window-panel][data-focused="true"]'));
}

function isFocusBlockedElement(element: Element | null): boolean {
  return element instanceof HTMLElement && Boolean(element.closest(focusBlockingSelector()));
}

function isKeyboardBlockedElement(element: Element | null): boolean {
  return element instanceof HTMLElement && Boolean(element.closest(`${focusBlockingSelector()}, button, a[href], [data-action]`));
}

function isFocusedHUDControl(element: Element | null): boolean {
  return (
    element instanceof HTMLElement &&
    Boolean(element.closest('.hud')) &&
    Boolean(element.closest('button, a[href], input, select, textarea, [data-action], [data-panel-toggle], [tabindex]'))
  );
}

function activeDocumentElement(): Element | null {
  return typeof document === 'undefined' ? null : document.activeElement;
}

function pointerBlockingSelector(): string {
  return '.hud, .auth-panel, .hud-modal, .hud-window, button, input, select, textarea, [role="dialog"]';
}

function focusBlockingSelector(): string {
  return '.auth-panel, .hud-modal, .hud-window, input, select, textarea, [role="dialog"], [contenteditable="true"]';
}
