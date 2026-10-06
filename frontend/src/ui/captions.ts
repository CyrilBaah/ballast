// How a video's captioning shows up in the UI (Feature 005
// contracts/wails-bindings.md, UI contract): one line under the upload's
// own status, always kept separate from it (FR-008), plus the one-time
// speech-model download prompt (FR-019).

import type { CaptionJob } from '../api/captions';
import { button } from './components';
import { formatBytes } from './format';

/** The caption line's text and tone for a job, or null for no line. */
export function captionStatus(c: CaptionJob | null | undefined): { text: string; tone: 'mute' | 'grass' | 'coral' } | null {
    if (!c) return null;
    const pct = c.progressPercent > 0 ? ` — ${c.progressPercent}%` : '';
    switch (c.status) {
        case 'done':
            return c.driveFileLink
                ? { text: 'Captions ready', tone: 'grass' }
                : { text: `Captions: ${c.note ?? 'nothing to caption'}`, tone: 'mute' };
        case 'failed':
            return { text: `Captions couldn't be made — ${c.note ?? 'unknown reason'}`, tone: 'coral' };
        case 'cancelled':
            return { text: c.note ? `Captions: ${c.note.toLowerCase()}` : 'Captions stopped', tone: 'mute' };
    }
    switch (c.phase) {
        case 'awaiting_consent':
            return { text: 'Captions: waiting for your answer', tone: 'mute' };
        case 'downloading_model':
            return { text: `Captions: downloading speech model${pct}`, tone: 'mute' };
        case 'extracting_audio':
            return { text: 'Captions: reading the audio', tone: 'mute' };
        case 'transcribing':
            return { text: `Captions: transcribing${pct}`, tone: 'mute' };
        case 'waiting_for_video':
            return { text: 'Captions ready on this Mac — waiting for the video to finish', tone: 'grass' };
        case 'uploading_captions':
            return { text: 'Captions: adding to Drive', tone: 'mute' };
    }
    return { text: 'Captions: waiting', tone: 'mute' };
}

/** The caption line; `inline` renders a span for use inside a button. */
export function captionLine(c: CaptionJob | null | undefined, inline = false): string {
    const s = captionStatus(c);
    if (!s) return '';
    const tag = inline ? 'span' : 'p';
    return `<${tag} class="caption-line fg-${s.tone === 'mute' ? 'faint' : s.tone}" data-caption-line style="display:block;margin:6px 0 0;font-size:11.5px">${escapeHtml(s.text)}</${tag}>`;
}

/** The one-time "download the speech model?" prompt (FR-019). */
export function captionConsentBanner(sizeBytes: number): string {
    return `<div class="card caption-consent" data-caption-consent-banner style="margin:0 0 16px;padding:14px 16px;display:flex;align-items:center;gap:14px;flex-wrap:wrap">
        <div style="flex:1;min-width:220px">
            <p style="margin:0;font-size:13px;font-weight:500">Make captions for your videos?</p>
            <p style="margin:4px 0 0;font-size:12px;color:var(--color-mute)">Ballast makes captions on this Mac, for free. It needs a one-time ${formatBytes(sizeBytes)} download of its speech model. Your uploads carry on either way.</p>
        </div>
        <div style="display:flex;gap:8px">
            ${button('Download and caption', { variant: 'primary', size: 'sm', attrs: 'data-caption-consent="yes"' })}
            ${button('Not now', { variant: 'outline', size: 'sm', attrs: 'data-caption-consent="no"' })}
        </div>
    </div>`;
}

function escapeHtml(s: string): string {
    return s.replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[ch]!);
}
