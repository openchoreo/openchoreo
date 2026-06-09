// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { type Page } from '@playwright/test';

// The YamlEditor component (plugins/openchoreo-react) wraps @uiw/react-codemirror
// (CodeMirror 6) and sets aria-label="YAML editor" on the .cm-content element.
// @uiw/react-codemirror attaches the CM6 EditorView to the .cm-content child
// element as .cmView.view — a stable library-internal path.

/**
 * Replace the entire content of the CodeMirror 6 YAML editor on the page.
 *
 * Uses the CM6 view.dispatch API for reliability. Falls back to select-all +
 * keyboard type if the view object is inaccessible (e.g. tree-shaken build).
 */
export async function cmSetContent(
  page: Page,
  yaml: string,
): Promise<void> {
  const editor = page.locator('.cm-editor');
  await editor.first().waitFor({ state: 'visible', timeout: 15_000 });

  const dispatched = await page.evaluate((content: string) => {
    const el = document.querySelector('.cm-content') as HTMLElement & {
      cmView?: { view: { state: { doc: { length: number } }; dispatch: (tr: unknown) => void } };
    };
    const view = el?.cmView?.view;
    if (!view) return false;
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: content },
    });
    return true;
  }, yaml);

  if (!dispatched) {
    const content = page.locator('[aria-label="YAML editor"]');
    await content.click();
    await page.keyboard.press('ControlOrMeta+a');
    await page.keyboard.type(yaml, { delay: 0 });
  }
}

/**
 * Read the current content of the CodeMirror 6 YAML editor.
 *
 * Waits for the editor to be visible and for the CM view to be attached
 * before reading, so callers don't race a slow render.
 */
export async function cmGetContent(
  page: Page,
  timeoutMs = 15_000,
): Promise<string> {
  await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: timeoutMs });

  const content = await page.evaluate((deadline: number) => {
    return new Promise<string>((resolve, reject) => {
      const check = () => {
        const el = document.querySelector('.cm-content') as HTMLElement & {
          cmView?: { view: { state: { doc: { toString: () => string } } } };
        };
        const text = el?.cmView?.view?.state?.doc?.toString();
        if (text !== undefined) return resolve(text);
        if (Date.now() > deadline) return reject(new Error('CodeMirror view not ready'));
        requestAnimationFrame(check);
      };
      check();
    });
  }, Date.now() + timeoutMs);

  return content;
}
