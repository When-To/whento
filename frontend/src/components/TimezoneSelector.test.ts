/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { lastEmit, mountWithI18n } from '@/test/harness';
import TimezoneSelector from './TimezoneSelector.vue';

async function search(query: string) {
  const wrapper = mountWithI18n(TimezoneSelector, { props: { modelValue: '' } });
  const input = wrapper.find('input');
  await input.trigger('focus');
  await input.setValue(query);
  return wrapper;
}

describe('TimezoneSelector', () => {
  // tzdata links Europe/Copenhagen to Europe/Berlin; Denmark must stay selectable (#206).
  it('offers zones tzdata folded into another country', async () => {
    const wrapper = await search('Copenhagen');
    const options = wrapper.findAll('button');
    expect(options).toHaveLength(1);
    await options[0]!.trigger('mousedown');
    expect(lastEmit(wrapper, 'update:modelValue')?.[0]).toBe('Europe/Copenhagen');
    wrapper.unmount();
  });

  it('leaves out old spellings of the same zone', async () => {
    const wrapper = await search('Asia/');
    const values = wrapper.findAll('button').map(b => b.text());
    expect(values.some(v => v.includes('Kolkata'))).toBe(true);
    expect(values.some(v => v.includes('Calcutta'))).toBe(false);
    wrapper.unmount();
  });
});
