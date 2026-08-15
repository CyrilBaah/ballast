// Journal (append-only activity log) — ported from Views.tsx's ActivityView.

import { icon } from '../components';
import { clockTime, relativeTime } from '../format';
import * as mock from '../mock-data';
import type { EventDTO } from '../mock-data';
import type { ViewCtx } from '../shell';

const LEVEL_TEXT: Record<string, string> = { info: 'fg-mute', success: 'fg-grass', warn: 'fg-tangerine', error: 'fg-coral' };
const LEVEL_DOT: Record<string, string> = { info: 'line', success: 'grass', warn: 'tangerine', error: 'coral' };

export function renderJournal(container: HTMLElement, _ctx: ViewCtx): void {
    const names = new Map(mock.uploads.map((u) => [u.id, u.name]));
    const groups: { day: string; items: EventDTO[] }[] = [];
    for (const e of mock.events) {
        const day = new Date(e.createdAt).toDateString();
        const last = groups[groups.length - 1];
        if (last && last.day === day) last.items.push(e);
        else groups.push({ day, items: [e] });
    }

    container.innerHTML = `
        <div class="scroll view-scroll journal-scroll">
            <div class="journal-inner">
                <span class="pill-badge tone-sky">${icon.pulse('icon')} Append-only</span>
                <h1 class="display" style="margin:12px 0 0;font-size:27px;line-height:1.2">Did it go through? Here's the receipt.</h1>
                <p style="margin:8px 0 0;max-width:560px;font-size:13.5px;line-height:1.6;color:var(--color-mute)">Every acknowledgement, every dropout, every recovery — written to disk in order, never rewritten.</p>

                <div class="journal-groups">
                    ${groups
                        .map(
                            (g) => `
                        <div>
                            <p class="eyebrow" style="margin-bottom:12px">${g.day}</p>
                            <div class="card journal-card">
                                <ol class="journal-list">
                                    ${g.items
                                        .map(
                                            (e, i) => `
                                        <li class="journal-item ${i < g.items.length - 1 ? 'bordered' : ''}">
                                            <span class="activity-dot ${LEVEL_DOT[e.level]}" style="margin-top:7px;height:8px;width:8px"></span>
                                            <div style="min-width:0;flex:1">
                                                <p class="${LEVEL_TEXT[e.level]}" style="margin:0;font-size:13px;line-height:1.5">${e.message}</p>
                                                ${e.detail ? `<p style="margin:2px 0 0;font-size:11.5px;line-height:1.5;color:var(--color-faint)">${e.detail}</p>` : ''}
                                                ${e.uploadId && names.get(e.uploadId) ? `<p class="mono" style="margin-top:6px;display:inline-block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:100%;border-radius:6px;background:var(--color-sand);padding:2px 8px;font-size:10.5px;color:var(--color-mute)">${names.get(e.uploadId)}</p>` : ''}
                                            </div>
                                            <span class="tnum" style="flex-shrink:0;text-align:right;font-size:10.5px;color:var(--color-faint)">
                                                <span style="display:block">${clockTime(e.createdAt)}</span>
                                                <span style="display:block">${relativeTime(e.createdAt)}</span>
                                            </span>
                                        </li>`,
                                        )
                                        .join('')}
                                </ol>
                            </div>
                        </div>`,
                        )
                        .join('')}
                    ${!mock.events.length ? `<p style="font-size:13px;color:var(--color-faint)">Nothing recorded yet.</p>` : ''}
                </div>
            </div>
        </div>
    `;
}
