// How a video's summary shows up in the UI (Feature 006
// contracts/wails-bindings.md, UI contract): one line under the caption
// line, kept separate from the upload and the captions (FR-011), plus the
// one-time summary-model prompt (FR-013).

import type { SummaryJob } from '../api/summaries';
import { button } from './components';
import { formatBytes } from './format';

export function summaryStatus(s: SummaryJob | null | undefined): { text: string; tone: 'mute' | 'grass' | 'coral' } | null {
    if (!s) return null;
    const pct = s.progressPercent > 0 ? ` — ${s.progressPercent}%` : '';
    switch (s.status) {
        case 'done':
            return { text: 'Summary ready', tone: 'grass' };
        case 'failed':
            return { text: `Summary couldn't be made — ${s.note ?? 'unknown reason'}`, tone: 'coral' };
        case 'cancelled':
            return { text: s.note ? `Summary: ${s.note.toLowerCase()}` : 'Summary stopped', tone: 'mute' };
    }
    switch (s.phase) {
        case 'waiting_for_captions':
            return { text: 'Summary: waiting for captions', tone: 'mute' };
        case 'awaiting_consent':
            return { text: 'Summary: waiting for your answer', tone: 'mute' };
        case 'downloading_model':
            return { text: `Summary: downloading summary model${pct}`, tone: 'mute' };
        case 'waiting_for_engine':
            return { text: 'Summary: waiting for captions to finish on this Mac', tone: 'mute' };
        case 'summarising':
            return { text: `Summary: writing${pct}`, tone: 'mute' };
        case 'waiting_for_video':
            return { text: 'Summary ready on this Mac — waiting for the video', tone: 'grass' };
        case 'uploading_summary':
            return { text: 'Summary: adding to Drive', tone: 'mute' };
    }
    return { text: 'Summary: waiting', tone: 'mute' };
}

export function summaryLine(s: SummaryJob | null | undefined, inline = false): string {
    const st = summaryStatus(s);
    if (!st) return '';
    const tag = inline ? 'span' : 'p';
    return `<${tag} class="summary-line fg-${st.tone === 'mute' ? 'faint' : st.tone}" data-summary-line style="display:block;margin:4px 0 0;font-size:11.5px">${escapeHtml(st.text)}</${tag}>`;
}

export function summaryConsentBanner(sizeBytes: number): string {
    return `<div class="card summary-consent" data-summary-consent-banner style="margin:0 0 16px;padding:14px 16px;display:flex;align-items:center;gap:14px;flex-wrap:wrap">
        <div style="flex:1;min-width:220px">
            <p style="margin:0;font-size:13px;font-weight:500">Write summaries of your videos?</p>
            <p style="margin:4px 0 0;font-size:12px;color:var(--color-mute)">Ballast writes summaries on this Mac, for free. It needs a one-time ${formatBytes(sizeBytes)} download of its summary model. Your uploads and captions carry on either way.</p>
        </div>
        <div style="display:flex;gap:8px">
            ${button('Download and summarise', { variant: 'primary', size: 'sm', attrs: 'data-summary-consent="yes"' })}
            ${button('Not now', { variant: 'outline', size: 'sm', attrs: 'data-summary-consent="no"' })}
        </div>
    </div>`;
}

function escapeHtml(s: string): string {
    return s.replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[ch]!);
}
