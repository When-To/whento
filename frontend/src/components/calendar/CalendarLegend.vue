<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

/**
 * What the colours mean.
 *
 * Each of the three previous calendar views carried its own legend; the rewrite
 * replaced all three and brought none of them back, leaving a grid whose whole
 * vocabulary — filled, outlined, ringed, edge-marked — was undocumented on screen.
 *
 * There is one legend now, rendered once by the parent, and its swatches are drawn from
 * the same custom properties as the cells they explain, so the two cannot drift apart.
 */

interface Props {
  /** The week grid paints collective coverage; the month and list views do not. */
  displayMode?: 'month' | 'week';
}

const props = withDefaults(defineProps<Props>(), { displayMode: 'month' });

const { t } = useI18n();

type SwatchKind = 'own' | 'shared' | 'threshold' | 'coverage' | 'holiday' | 'eve' | 'disabled';

const items = computed<{ kind: SwatchKind; label: string }[]>(() => {
  const entries: { kind: SwatchKind; label: string }[] = [
    { kind: 'own', label: t('calendar.legend.own') },
    { kind: 'threshold', label: t('calendar.legend.threshold') },
    // The violet outline. Only meaningful once participants are picked in the list
    // below the calendar, but listing it unconditionally is what makes the selection
    // discoverable in the first place.
    { kind: 'shared', label: t('calendar.legend.shared') },
  ];

  if (props.displayMode === 'week') {
    entries.push({ kind: 'coverage', label: t('calendar.legend.coverage') });
  }

  entries.push(
    { kind: 'holiday', label: t('calendar.legend.holiday') },
    { kind: 'eve', label: t('calendar.legend.holidayEve') },
    { kind: 'disabled', label: t('calendar.legend.disabled') }
  );

  return entries;
});
</script>

<template>
  <section class="cal-legend" :aria-label="t('calendar.legend.title')">
    <span v-for="item in items" :key="item.kind" class="cal-legend-item">
      <span class="cal-legend-swatch" :data-kind="item.kind" aria-hidden="true" />
      {{ item.label }}
    </span>

    <!-- The gauge is the one sign that is not a colour, so it gets its own entry. -->
    <span class="cal-legend-item">
      <span class="cal-legend-gauge" aria-hidden="true"><span style="width: 60%" /></span>
      {{ t('calendar.legend.gauge') }}
    </span>
  </section>
</template>
