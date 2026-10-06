// Transfers list, TransferCard and DetailPanel — ported from the Tether
// prototype's AppShell.tsx (transfers view), TransferCard.tsx and
// DetailPanel.tsx, now wired to real uploads via ../live.ts. The backend
// has no manual pause, no chunk-level telemetry, no per-file throughput
// history, and no checksum/session data exposed to the frontend, so those
// sections and the Pause/Verify actions from the original mock design are
// gone rather than shown with fabricated numbers.

import { icon, kindIcon, KIND_TONE, statusChip, progressBar, sparkline, ring, button } from '../components';
import { formatBytes, formatDuration, formatPercent, formatSpeed, relativeTime, clockTime } from '../format';
import * as live from '../live';
import type { LiveUpload } from '../live';
import { toPlainLanguage } from '../../errors';
import type { ViewCtx } from '../shell';

const FILTERS: { id: 'active' | 'all' | 'secured'; label: string }[] = [
    { id: 'active', label: 'In flight' },
    { id: 'all', label: 'Everything' },
    { id: 'secured', label: 'Delivered' },
];

// The one decision still worth a user's attention: the file on disk is no
// longer the one they picked, so sending it would put different contents
// in Drive under the same name. A session Drive dropped never reaches this
// state -- there is nothing left to resume against and only one way for
// the file to land, so the app opens a fresh session and carries on by
// itself (app.go's restartExpiredSession).
const CONFIRMATION_COPY: Record<string, string> = {
    file_changed: 'The local file changed since this upload started.',
};

export function renderTransfers(container: HTMLElement, ctx: ViewCtx): void {
    const uploads = live.orderedUploads();
    const s = live.liveStats();
    const remaining = s.totalBytes - s.confirmedBytes;

    const visible = uploads.filter((u) => {
        if (ctx.filter === 'all') return true;
        if (ctx.filter === 'secured') return u.status === 'completed' || u.status === 'canceled';
        return u.status !== 'completed' && u.status !== 'canceled';
    });

    const selected = uploads.find((u) => u.id === ctx.selectedUploadId) ?? null;

    container.innerHTML = `
        <div style="display:flex;height:100%;min-width:0">
            <div class="scroll view-scroll transfers-scroll">
                <header class="transfers-header">
                    <div>
                        <h1 class="display" style="margin:0;font-size:26px;line-height:1.2">Transfers</h1>
                        <p style="margin:6px 0 0;font-size:13px;color:var(--color-mute)">
                            <span class="fg-grass" style="font-weight:500">${formatBytes(s.confirmedBytes)}</span>
                            confirmed by Google Drive ${remaining > 0 ? `· ${formatBytes(remaining)} still to go` : ''}
                        </p>
                    </div>
                    <div style="display:flex;align-items:center;gap:8px">
                        ${button(`${icon.plus()} Choose files`, {
                            variant: 'primary',
                            attrs: `id="transfers-pick"`,
                        })}
                    </div>
                </header>

                <div class="transfers-tiles">
                    ${tile('Safe on Drive', formatBytes(s.confirmedBytes), `${s.completedCount} file${s.completedCount === 1 ? '' : 's'} fully verified`, 'grass', icon.shield())}
                    ${tile('In your queue', formatBytes(s.totalBytes), `${s.activeCount} active now`, 'sky', icon.refresh())}
                    <div class="card transfers-link-tile">
                        <div style="display:flex;align-items:center;justify-content:space-between">
                            <p class="eyebrow">Live speed</p>
                        </div>
                        <p class="tnum" style="margin:6px 0 0;font-size:19px;font-weight:500">${formatSpeed(s.throughputBps)}</p>
                        <div style="margin-top:4px;height:28px">${sparkline(live.netHistory, { color: s.throughputBps > 0 ? '#12a970' : '#a1978e', height: 28 })}</div>
                    </div>
                </div>

                <div class="transfers-filter-row">
                    <div class="filter-tabs">
                        ${FILTERS.map((f) => `<button type="button" class="filter-tab ${ctx.filter === f.id ? 'active' : ''}" data-filter="${f.id}">${f.label}</button>`).join('')}
                    </div>
                </div>

                <div class="transfers-list">
                    ${
                        visible.length
                            ? visible.map((u) => transferCard(u, u.id === ctx.selectedUploadId)).join('')
                            : `<div class="card empty-state">
                                <span class="empty-state-icon tone-butter">${icon.folder('icon icon-lg')}</span>
                                <p class="display" style="margin:16px 0 0;font-size:18px">Nothing in this lane</p>
                                <p style="margin:6px auto 0;max-width:380px;font-size:12.5px;line-height:1.6;color:var(--color-mute)">Pick a file and Ballast will carry it across every dropout until Drive confirms the final byte.</p>
                                <div style="margin-top:20px">${button(`${icon.plus()} Choose files`, { variant: 'primary', attrs: 'id="transfers-empty-pick"' })}</div>
                            </div>`
                    }
                </div>
            </div>
            <div id="detail-panel-slot"></div>
        </div>
    `;

    container.querySelector('#transfers-pick')?.addEventListener('click', () => ctx.openPicker());
    container.querySelector('#transfers-empty-pick')?.addEventListener('click', () => ctx.openPicker());
    container.querySelectorAll<HTMLButtonElement>('[data-filter]').forEach((el) => {
        el.addEventListener('click', () => ctx.setFilter(el.dataset.filter as 'active' | 'all' | 'secured'));
    });
    container.querySelectorAll<HTMLElement>('[data-select]').forEach((el) => {
        el.addEventListener('click', (e) => {
            if ((e.target as HTMLElement).closest('[data-card-action]')) return;
            const id = el.dataset.select!;
            ctx.select(ctx.selectedUploadId === id ? null : id);
        });
    });
    container.querySelectorAll<HTMLElement>('[data-card-action]').forEach((el) => {
        el.addEventListener('click', (e) => {
            e.stopPropagation();
            void applyAction(el.dataset.cardAction!, el.dataset.cardId!, ctx);
        });
    });

    if (selected) {
        const slot = container.querySelector<HTMLElement>('#detail-panel-slot')!;
        slot.innerHTML = detailPanel(selected);
        slot.querySelector('#detail-close')?.addEventListener('click', () => ctx.select(null));
        slot.querySelectorAll<HTMLElement>('[data-detail-action]').forEach((el) => {
            el.addEventListener('click', () => void applyAction(el.dataset.detailAction!, selected.id, ctx));
        });
    }
}

async function applyAction(action: string, id: string, ctx: ViewCtx) {
    try {
        if (action === 'cancel') {
            await live.cancelUpload(id);
            ctx.rerender();
        } else if (action === 'restart') {
            await live.confirmRestartUpload(id);
            ctx.showToast('Restarting from the beginning.');
            ctx.rerender();
        } else if (action === 'open-drive') {
            const u = live.orderedUploads().find((x) => x.id === id);
            if (u?.driveFileLink) window.open(u.driveFileLink, '_blank', 'noopener,noreferrer');
        } else if (action === 'delete') {
            await live.deleteUpload(id);
            if (ctx.selectedUploadId === id) ctx.select(null);
            ctx.rerender();
        } else if (action === 'retry') {
            await live.retryUpload(id);
            ctx.rerender();
        }
    } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        ctx.showToast(toPlainLanguage(message));
    }
}

function tile(label: string, value: string, sub: string, tone: string, iconSvg: string): string {
    return `<div class="card transfers-tile">
        <div style="display:flex;align-items:center;justify-content:space-between">
            <p class="eyebrow">${label}</p>
            <span class="transfers-tile-icon tone-${tone}">${iconSvg}</span>
        </div>
        <p class="tnum fg-${tone}" style="margin:6px 0 0;font-size:19px;font-weight:500">${value}</p>
        <p style="margin:2px 0 0;font-size:11px;color:var(--color-faint)">${sub}</p>
    </div>`;
}

function cardActions(u: LiveUpload, size: 'icon' | 'sm'): string {
    if (u.status === 'awaiting_confirmation') {
        return `${button(`${icon.play('icon')}${size === 'sm' ? ' Send the file as it is now' : ''}`, { variant: size === 'sm' ? 'primary' : 'ghost', size, attrs: `data-card-action="restart" data-card-id="${u.id}" data-detail-action="restart" title="Send the file as it is now"` })}${button(icon.x('icon'), { variant: size === 'sm' ? 'outline' : 'ghost', size, attrs: `data-card-action="cancel" data-card-id="${u.id}" data-detail-action="cancel" title="Cancel"` })}`;
    }
    if (u.status === 'queued' || u.status === 'uploading' || u.status === 'reconnecting') {
        return button(icon.x('icon'), { variant: 'ghost', size, attrs: `data-card-action="cancel" data-card-id="${u.id}" data-detail-action="cancel" title="Cancel"` });
    }
    if (u.status === 'completed' && u.driveFileLink) {
        return button(`${icon.drive('icon')}${size === 'sm' ? ' View in Drive' : ''}`, { variant: size === 'sm' ? 'soft' : 'ghost', size, attrs: `data-card-action="open-drive" data-card-id="${u.id}" data-detail-action="open-drive" title="View in Drive"` });
    }
    const deleteBtn = button(`${icon.trash('icon')}${size === 'sm' ? ' Delete' : ''}`, { variant: size === 'sm' ? 'outline' : 'ghost', size, attrs: `data-card-action="delete" data-card-id="${u.id}" data-detail-action="delete" title="Delete"` });
    if (u.status === 'canceled') {
        const retryBtn = button(`${icon.refresh('icon')}${size === 'sm' ? ' Retry' : ''}`, { variant: size === 'sm' ? 'primary' : 'ghost', size, attrs: `data-card-action="retry" data-card-id="${u.id}" data-detail-action="retry" title="Retry"` });
        return `${retryBtn}${deleteBtn}`;
    }
    if (u.status === 'failed') {
        return deleteBtn;
    }
    return '';
}

function transferCard(u: LiveUpload, selected: boolean): string {
    const pct = u.sizeBytes > 0 ? u.bytesConfirmed / u.sizeBytes : 0;
    const remaining = u.sizeBytes - u.bytesConfirmed;
    const eta = u.throughputBps > 0 ? (remaining / u.throughputBps) * 1000 : 0;
    const live_ = u.status === 'uploading';
    const holding = u.status === 'reconnecting';
    const tone = KIND_TONE[u.kind] ?? KIND_TONE.archive;

    return `
    <div class="transfer-card ${selected ? 'selected' : ''}" data-select="${u.id}">
        <div style="display:flex;align-items:flex-start;gap:14px">
            <div class="transfer-card-icon ${holding ? 'tone-tangerine' : tone}">${kindIcon(u.kind, 'icon icon-lg')}</div>
            <div style="min-width:0;flex:1">
                <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:12px">
                    <div style="min-width:0">
                        <p class="transfer-card-name">${u.name}</p>
                        <p class="transfer-card-sub">
                            ${icon.drive('icon')} ${u.destination}
                            <span class="fg-faint">•</span>
                            <span class="tnum">${formatBytes(u.sizeBytes)}</span>
                        </p>
                    </div>
                    ${statusChip(u.status, true)}
                </div>

                <div style="margin-top:14px">${progressBar(pct, 0, { animate: live_, tone: holding || u.status === 'awaiting_confirmation' ? 'tangerine' : 'grass' })}</div>

                <div class="transfer-card-meta">
                    <div class="transfer-card-meta-left tnum">
                        <span style="font-weight:600;color:var(--color-ink)">${formatPercent(pct * 100)}</span>
                        <span class="fg-faint">${formatBytes(u.bytesConfirmed)} / ${formatBytes(u.sizeBytes)}</span>
                        ${live_ ? `<span class="fg-grass" style="font-weight:500">${formatSpeed(u.throughputBps)}</span>` : ''}
                        ${live_ && eta > 0 ? `<span class="fg-faint">${formatDuration(eta)} left</span>` : ''}
                        ${holding ? `<span class="fg-tangerine" style="font-weight:500">reconnecting…</span>` : ''}
                        ${u.status === 'awaiting_confirmation' ? `<span class="fg-tangerine" style="font-weight:500">${toPlainLanguage(CONFIRMATION_COPY[u.awaitingConfirmationReason ?? ''] ?? 'Needs your decision.')}</span>` : ''}
                        ${u.status === 'completed' ? `<span class="fg-grass">confirmed by Drive</span>` : ''}
                        ${u.status === 'failed' && u.failureReason ? `<span class="fg-coral">${toPlainLanguage(u.failureReason)}</span>` : ''}
                    </div>
                    <div class="transfer-card-actions">${cardActions(u, 'icon')}</div>
                </div>
            </div>
        </div>
    </div>`;
}

function detailPanel(u: LiveUpload): string {
    const pct = u.sizeBytes > 0 ? u.bytesConfirmed / u.sizeBytes : 0;
    const remaining = u.sizeBytes - u.bytesConfirmed;
    const eta = u.throughputBps > 0 ? (remaining / u.throughputBps) * 1000 : 0;
    const live_ = u.status === 'uploading';
    const tone = KIND_TONE[u.kind] ?? KIND_TONE.archive;
    const ringColor = u.status === 'reconnecting' || u.status === 'awaiting_confirmation' ? '#f97316' : u.status === 'failed' ? '#f43f5e' : '#12a970';

    return `
    <aside class="detail-panel">
        <div class="mesh-soft detail-panel-head">
            <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:12px">
                <div style="display:flex;min-width:0;gap:12px">
                    <div class="detail-panel-icon ${tone}">${kindIcon(u.kind, 'icon icon-lg')}</div>
                    <div style="min-width:0">
                        <p style="margin:0;font-size:13.5px;font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${u.name}</p>
                        <p style="margin:2px 0 0;font-size:11.5px;color:var(--color-faint);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${u.destination}</p>
                    </div>
                </div>
                ${button(icon.x(), { variant: 'ghost', size: 'icon', attrs: 'id="detail-close" title="Close"' })}
            </div>

            <div style="margin-top:20px;display:flex;align-items:center;gap:16px">
                ${ring(pct, { size: 64, stroke: 6, color: ringColor, inner: `<span class="tnum" style="font-size:13px;font-weight:600">${Math.floor(pct * 100)}%</span>` })}
                <div style="min-width:0;flex:1">
                    ${statusChip(u.status)}
                    <p class="tnum" style="margin:8px 0 0;font-size:12px;color:var(--color-mute)"><span style="font-weight:500;color:var(--color-ink)">${formatBytes(u.bytesConfirmed)}</span> safely on Drive of ${formatBytes(u.sizeBytes)}</p>
                    <div style="margin-top:8px">${progressBar(pct, 0, { animate: live_, height: 7, tone: u.status === 'reconnecting' || u.status === 'awaiting_confirmation' ? 'tangerine' : 'grass' })}</div>
                </div>
            </div>

            ${
                u.status === 'failed' && u.failureReason
                    ? `<p style="margin-top:12px;font-size:12px;color:var(--color-coral)">${toPlainLanguage(u.failureReason)}</p>`
                    : u.status === 'awaiting_confirmation'
                      ? `<p style="margin-top:12px;font-size:12px;color:var(--color-tangerine)">${toPlainLanguage(CONFIRMATION_COPY[u.awaitingConfirmationReason ?? ''] ?? 'This upload needs a decision.')} Send it as it is now, or cancel it.</p>`
                      : `<p style="margin-top:12px;font-size:12px;font-style:italic;color:var(--color-mute)">"${STATUS_WARM[u.status]}"</p>`
            }

            <div style="margin-top:14px;display:flex;flex-wrap:wrap;gap:8px">${cardActions(u, 'sm')}</div>
        </div>

        <div class="scroll detail-panel-body">
            <section>
                <h3 class="eyebrow">Session details</h3>
                <div class="card-flat detail-rows">
                    ${row('Destination', u.destination)}
                    ${row('Started', `${clockTime(u.startedAt)} · ${relativeTime(u.startedAt)}`)}
                    ${live_ && eta > 0 ? row('Estimated time left', formatDuration(eta)) : ''}
                    ${row('Progress', formatPercent(pct * 100))}
                </div>
            </section>
        </div>
    </aside>`;
}

const STATUS_WARM: Record<string, string> = {
    queued: 'Waiting its turn',
    uploading: 'Flowing nicely',
    reconnecting: 'Holding your place',
    completed: 'Done and verified',
    canceled: 'Stopped by you',
};

function row(label: string, value: string): string {
    return `<div class="detail-row">
        <span class="fg-faint" style="font-size:11.5px;flex-shrink:0">${label}</span>
        <span class="tnum" style="font-size:12px;color:rgba(36,31,27,0.85);text-align:right;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${value}</span>
    </div>`;
}
